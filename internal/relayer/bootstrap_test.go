package relayer

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmttypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func (h *headerJobRPC) launchData(enabled bool) []byte {
	data := append(headerJobDisc("account", "LaunchConfig"), 1)
	payer := h.s.payer.PublicKey()
	data = append(data, payer[:]...)
	data = append(data, h.s.deployment.Mint[:]...)
	flag := byte(0)
	if enabled {
		flag = 1
	}
	data = append(data, flag)
	data = binary.LittleEndian.AppendUint64(data, h.slot)
	return append(data, 0, 0)
}

func (h *headerJobRPC) applyBootstrap(program solana.PublicKey, data []byte, key func(int) solana.PublicKey) {
	if bytes.Equal(data[:8], headerJobDisc("global", "initialize")) {
		switch program {
		case h.f.AccessManager:
			h.accounts[h.f.AccessState] = headerJobAccount(program, h.accessData())
		case h.s.deployment.Program:
			h.accounts[h.s.deployment.Config] = headerJobAccount(program, h.launchData(false))
		case h.f.Program:
			clientSize := 4 + len(h.s.cfg.chainID) + 9*8
			clientData := append(headerJobDisc("account", "ClientState"), data[8:8+clientSize]...)
			state, err := bridge.DecodeClient(clientData)
			require.NoError(h.t, err)
			require.Equal(h.t, uint64(1), state.TrustLevelNumerator)
			require.Equal(h.t, uint64(3), state.TrustLevelDenominator)
			require.Equal(h.t, uint64(86400), state.TrustingPeriod)
			require.Equal(h.t, uint64(1814400), state.UnbondingPeriod)
			require.Equal(h.t, uint64(10), state.MaxClockDrift)
			require.Equal(h.t, bridge.Height{}, state.FrozenHeight)
			require.Equal(h.t, uint64(1), state.LatestHeight.RevisionNumber)
			h.latest = state.LatestHeight.RevisionHeight
			h.accounts[key(0)] = headerJobAccount(program, clientData)
			consensus := binary.LittleEndian.AppendUint64(headerJobDisc("account", "ConsensusStateStore"), h.latest)
			consensus = append(consensus, data[8+clientSize:8+clientSize+72]...)
			h.accounts[key(1)] = headerJobAccount(program, consensus)
		}
	} else if bytes.Equal(data[:8], headerJobDisc("global", "configure_bridge")) {
		bridgeData := append(headerJobDisc("account", "BridgeConfig"), h.f.Program[:]...)
		bridgeData = append(bridgeData, data[8:]...)
		bridgeData = append(bridgeData, make([]byte, 16)...)
		configured, err := bridge.DecodeBridge(bridgeData)
		require.NoError(h.t, err)
		require.Equal(h.t, uint64(1_000_000_000_000), configured.Policy.PerTransferMax)
		require.Equal(h.t, uint64(10_000_000_000_000), configured.Policy.DailyMintCap)
		require.Empty(h.t, configured.Policy.Vaults)
		require.Equal(h.t, h.f.Program, key(5))
		h.accounts[h.s.deployment.Bridge] = headerJobAccount(program, bridgeData)
	} else if bytes.Equal(data[:8], headerJobDisc("global", "set_bridge_enabled")) {
		h.accounts[h.s.deployment.Config] = headerJobAccount(program, h.launchData(data[8] != 0))
	} else if bytes.Equal(data[:8], headerJobDisc("global", "grant_role")) {
		require.Equal(h.t, uint64(1), binary.LittleEndian.Uint64(data[8:]))
		require.Equal(h.t, h.s.payer.PublicKey(), solana.PublicKeyFromBytes(data[16:]))
	}
}

func (h *headerJobRPC) attachNative(t *testing.T, target int64, waitForFour bool) header.Encoding {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("header", "testdata", "template.json"))
	require.NoError(t, err)
	source, err := header.ParseSource(raw)
	require.NoError(t, err)
	source.Signed.Header.Height = target
	source.Signed.Header.Time = time.Now().UTC().Truncate(time.Second)
	source.TrustedHeight = uint64(target - 1)
	source.Revision = 1
	source, err = header.Fixture(source, 4, 4, false)
	require.NoError(t, err)
	encoding, err := header.Encode(source)
	require.NoError(t, err)
	h.packet, err = header.RuntimePacket(encoding)
	require.NoError(t, err)
	h.clock = source.Signed.Header.Time.Unix()
	h.setClock()
	trusted, err := h.f.ConsensusAt(h.latest)
	require.NoError(t, err)
	h.accounts[trusted] = headerJobAccount(h.f.Program, headerJobConsensus(h.latest, h.packet))
	var mu sync.Mutex
	statusCalls := 0
	height := target + 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var call struct {
			ID     json.RawMessage            `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&call))
		param := func(name string) string {
			var value string
			require.NoError(t, json.Unmarshal(call.Params[name], &value))
			return value
		}
		var result any
		switch call.Method {
		case "status":
			statusCalls++
			height = target + 1
			if waitForFour && statusCalls == 1 {
				height = 3
			}
			result = &ctypes.ResultStatus{
				NodeInfo: p2p.DefaultNodeInfo{
					Network: h.s.cfg.chainID,
				},
				SyncInfo: ctypes.SyncInfo{
					LatestBlockHeight: height,
					LatestBlockTime:   source.Signed.Header.Time,
				},
			}
		case "abci_info":
			result = &ctypes.ResultABCIInfo{
				Response: abci.ResponseInfo{
					LastBlockHeight: height,
				},
			}
		case "block":
			result = &ctypes.ResultBlock{
				Block: &cmttypes.Block{
					Header: cmttypes.Header{
						ChainID: h.s.cfg.chainID,
						Height:  height,
						Time:    source.Signed.Header.Time,
					},
				},
			}
		case "abci_query":
			var value proto.Message
			switch param("path") {
			case "/akash.sink.v1.Query/State":
				value = &sink.QueryStateResponse{
					Epoch: 1,
					SendUsage: sink.EpochUsage{
						Epoch:  1,
						Amount: sdk.NewInt64Coin("uakt", 0),
					},
					RefundUsage: sink.EpochUsage{
						Epoch:  1,
						Amount: sdk.NewInt64Coin("uakt", 0),
					},
				}
			case "/akash.sink.v1.Query/Params":
				value = &sink.QueryParamsResponse{}
			case "/akash.sink.v1.Query/Reporters":
				value = &sink.QueryReportersResponse{
					Reporters: []sink.ReporterPolicy{
						{
							Version:   1,
							Threshold: sdkmath.LegacyOneDec(),
						},
					},
				}
			}
			raw, err := proto.Marshal(value)
			require.NoError(t, err)
			result = &ctypes.ResultABCIQuery{
				Response: abci.ResponseQuery{
					Height: height,
					Value:  raw,
				},
			}
		case "commit":
			require.Equal(t, strconv.FormatInt(target, 10), param("height"))
			result = &ctypes.ResultCommit{
				SignedHeader:    *source.Signed,
				CanonicalCommit: true,
			}
		case "validators":
			queryHeight, err := strconv.ParseInt(param("height"), 10, 64)
			require.NoError(t, err)
			result = &ctypes.ResultValidators{
				BlockHeight: queryHeight,
				Validators:  source.Validators,
				Count:       len(source.Validators),
				Total:       len(source.Validators),
			}
		default:
			t.Errorf("unexpected native RPC %s", call.Method)
		}
		raw, err := cmtjson.Marshal(result)
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      call.ID,
			"result":  json.RawMessage(raw),
		}))
	}))
	t.Cleanup(server.Close)
	h.s.native, err = native.New(server.URL, h.s.cfg.chainID, time.Hour)
	require.NoError(t, err)
	return encoding
}

func TestHeaderJobBootstrapFreshAndPreservedPause(t *testing.T) {
	h := newHeaderJobFixture(t)
	expected := h.attachNative(t, 11, true)
	delete(h.accounts, h.s.deployment.Bridge)
	delete(h.accounts, h.s.deployment.Config)
	delete(h.accounts, h.f.ClientState)
	delete(h.accounts, h.f.AccessState)
	require.NoError(t, h.s.bootstrap(context.Background()))
	require.Len(t, h.stages, 6)
	checkpoint, err := os.ReadFile(filepath.Join(h.s.cfg.stateDir, "checkpoint.json"))
	require.NoError(t, err)
	var stored map[string]any
	require.NoError(t, json.Unmarshal(checkpoint, &stored))
	require.Equal(t, float64(11), stored["height"])
	require.Equal(t, hex.EncodeToString(expected.Root), stored["root"])
	require.Equal(t, hex.EncodeToString(expected.BlockHash[:]), stored["blockHash"])
	require.Contains(t, h.out.String(), `"event":"trusted-local-checkpoint"`)
	require.Contains(t, h.out.String(), `"enabled":true`)
	for _, call := range h.calls {
		if call.Method == "sendTransaction" {
			require.Contains(t, string(call.Params[1]), `"skipPreflight":false`)
			require.Contains(t, string(call.Params[1]), `"preflightCommitment":"confirmed"`)
		}
		require.NotEqual(t, "simulateTransaction", call.Method)
	}
	h.accounts[h.s.deployment.Config] = headerJobAccount(h.s.deployment.Program, h.launchData(false))
	require.NoError(t, h.s.bootstrap(context.Background()))
	require.Len(t, h.stages, 6)
	require.Contains(t, h.out.String(), `"enabled":false`)
}

func TestHeaderJobBootstrapClockWaitCancelAndOutage(t *testing.T) {
	for _, mode := range []string{"canceled", "timeout", "outage"} {
		t.Run(mode, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.accounts[h.s.deployment.Config] = headerJobAccount(h.s.deployment.Program, h.launchData(false))
			delete(h.accounts, h.s.deployment.Bridge)
			h.clock = 0
			h.setClock()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "outage" {
				h.failAddress = solana.SysVarClockPubkey
			} else if mode == "canceled" {
				h.s.deps.sleep = func(context.Context, time.Duration) error {
					cancel()
					return ctx.Err()
				}
			}
			err := h.s.bootstrap(ctx)
			require.Error(t, err)
			if mode == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			} else if mode == "timeout" {
				require.ErrorContains(t, err, "Solana clock has not reached")
			}
			require.Empty(t, h.stages)
		})
	}
}

func TestHeaderJobUpdateUsesNativeEncoderAndTamper(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		t.Run(strconv.FormatBool(tamper), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			expected := h.attachNative(t, 11, false)
			// Stop after active persistence; inspect the exact packet before any payment.
			h.failMethod = "getBalance"
			err := h.s.update(context.Background(), 11, tamper, nil)
			require.Error(t, err)
			active, err := h.s.store.loadActive()
			require.NoError(t, err)
			require.NotNil(t, active)
			packet, err := runtimeHeaderPacket(active.Packet, h.s.cfg.chainID)
			require.NoError(t, err)
			want := append([]byte{}, expected.Header...)
			if tamper {
				want[bytes.Index(want, expected.Root)] ^= 1
			}
			require.Equal(t, want, packet.Header)
			require.Equal(t, expected.Root, packet.Root[:])
			require.Equal(t, expected.Receipts, packet.Receipts)
			require.Empty(t, h.stages)
		})
	}
}
