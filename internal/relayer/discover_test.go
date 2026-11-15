package relayer

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/gogoproto/proto"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func planningService(t *testing.T) *service {
	t.Helper()
	directory := durableTestDirectory(t)
	identity := durableIdentity(t)
	store, err := newStateStore(directory, identity.ChainID, identity.Genesis, identity.ProgramID, identity.Payer, nil)
	require.NoError(t, err)
	return &service{
		store: store,
		deps: dependencies{
			now: time.Now,
		},
	}
}

func planningTransfer(sequence uint64) sink.TransferState {
	return sink.TransferState{
		ID: sink.ID{
			Owner:    sdk.AccAddress(bytes.Repeat([]byte{7}, 20)).String(),
			Height:   10,
			Sequence: sequence,
		},
		Data: sink.RecordData{
			Dest:   solana.SystemProgramID.String(),
			Amount: sdk.NewInt64Coin("uakt", 123),
		},
		Settlement: sink.SettlementData{
			Round: 2,
			Policy: sink.ReporterPolicy{
				Version:   7,
				Threshold: sdkmath.LegacyOneDec(),
			},
		},
		Status: sink.StatusTransferPending,
	}
}

func planningReply(t *testing.T, response proto.Message, height int64) abci.ResponseQuery {
	t.Helper()
	data, err := proto.Marshal(response)
	require.NoError(t, err)
	return abci.ResponseQuery{
		Height: height,
		Value:  data,
	}
}

func planningNative(t *testing.T, handle func(string, int64, []byte, bool) (abci.ResponseQuery, error)) *native.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Path, Height, Data string
				Prove              bool
			} `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "abci_query", request.Method)
		height, err := strconv.ParseInt(request.Params.Height, 10, 64)
		require.NoError(t, err)
		data, err := hex.DecodeString(strings.TrimPrefix(request.Params.Data, "0x"))
		require.NoError(t, err)
		response, failure := handle(request.Params.Path, height, data, request.Params.Prove)
		body := map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
		}
		if failure != nil {
			body["error"] = map[string]any{
				"code":    -32603,
				"message": failure.Error(),
			}
		} else {
			encoded, err := cmtjson.Marshal(&ctypes.ResultABCIQuery{Response: response})
			require.NoError(t, err)
			body["result"] = json.RawMessage(encoded)
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(body))
	}))
	t.Cleanup(server.Close)
	client, err := native.NewWithHTTP(server.URL, "planning-test", time.Minute, &http.Client{Timeout: 2 * time.Second})
	require.NoError(t, err)
	return client
}

func planningSolana(t *testing.T, handle func([]solana.PublicKey) ([]*rpc.Account, error)) *bridge.Reader {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "getMultipleAccounts", request.Method)
		var keys []solana.PublicKey
		require.NoError(t, json.Unmarshal(request.Params[0], &keys))
		accounts, err := handle(keys)
		response := map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
		}
		if err != nil {
			response["error"] = map[string]any{
				"code":    -32603,
				"message": err.Error(),
			}
		} else {
			response["result"] = map[string]any{
				"context": map[string]any{"slot": 9},
				"value":   accounts,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	t.Cleanup(server.Close)
	client := rpc.New(server.URL)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return bridge.NewReader(client, solana.PublicKey(bytes.Repeat([]byte{1}, 32)), "planning-test", nil)
}

func TestDiscoveryPrioritiesHistoryFailureAndCheckpoint(t *testing.T) {
	for _, failure := range []string{"", "history", "status", "history-after-page", "history-and-status"} {
		t.Run(failure, func(t *testing.T) {
			s := planningService(t)
			var events bytes.Buffer
			s.deps.streams.Err = &events
			values := make(map[uint64]sink.TransferState)
			for i := uint64(1); i <= 9; i++ {
				values[i] = planningTransfer(i)
			}
			disputed := values[4]
			disputed.Settlement.Hold = sink.HoldConflict
			refund := values[3]
			refund.Status = sink.StatusRefundPending
			voided := values[5]
			voided.Status = sink.StatusVoided
			checkpoint := &native.Cursor{
				Kind:   "tail",
				Key:    []byte("checkpoint"),
				LastID: values[1].ID,
			}
			require.NoError(t, s.store.saveDiscovery(checkpoint))
			before, err := os.ReadFile(s.store.discoveryPath)
			require.NoError(t, err)
			planningProof(t, s, values[6], 20)
			// Parse order differs from lexical ID order. Saving is intentionally sorted by TS.
			idText := func(sequence uint64) string {
				value := values[sequence]
				return value.ID.String()
			}
			ledger := fmt.Sprintf(`{"version":2,"genesis":%q,"programId":%q,"transfers":{%q:{"spentLamports":0,"attempts":1},%q:{"spentLamports":0,"attempts":1},%q:{"spentLamports":0,"attempts":1,"completed":true},%q:{"spentLamports":0,"attempts":1}}}`, s.store.identity.Genesis, s.store.identity.ProgramID, idText(8), idText(7), idText(9), idText(6))
			require.NoError(t, atomicJSON(s.store.ledgerPath, []byte(ledger)))
			var mu sync.Mutex
			var requeued []sink.ID
			historyCalls := 0
			s.native = planningNative(t, func(path string, height int64, data []byte, prove bool) (abci.ResponseQuery, error) {
				mu.Lock()
				defer mu.Unlock()
				require.Equal(t, int64(100), height)
				require.False(t, prove)
				switch path {
				case "/akash.sink.v1.Query/Disputes":
					return planningReply(t, &sink.QueryDisputesResponse{
						Transfers:  []sink.TransferState{disputed},
						Pagination: &query.PageResponse{},
					}, height), nil
				case "/akash.sink.v1.Query/Transfer":
					var request sink.QueryTransferRequest
					require.NoError(t, request.Unmarshal(data))
					requeued = append(requeued, request.ID)
					return planningReply(t, &sink.QueryTransferResponse{Transfer: values[request.ID.Sequence]}, height), nil
				case "/akash.sink.v1.Query/Transfers":
					var request sink.QueryTransfersRequest
					require.NoError(t, request.Unmarshal(data))
					page := &sink.QueryTransfersResponse{
						Pagination: &query.PageResponse{},
					}
					switch request.Status {
					case sink.StatusRefundPending:
						page.Transfers = []sink.TransferState{refund}
					case sink.StatusVoided:
						page.Transfers = []sink.TransferState{voided}
					case sink.StatusTransferPending:
						page.Transfers = []sink.TransferState{values[4], values[2]}
					default:
						historyCalls++
						if failure == "history" || failure == "history-and-status" || (failure == "history-after-page" && historyCalls == 2) {
							return abci.ResponseQuery{}, errors.New("history offline")
						}
						require.Equal(t, checkpoint.Key, request.Pagination.Key)
						page.Transfers = []sink.TransferState{values[1], values[2], refund, values[4], voided, values[6], values[7], values[8], values[9]}
						if failure == "history-after-page" {
							page.Pagination.NextKey = []byte("next")
						}
					}
					return planningReply(t, page, height), nil
				default:
					return abci.ResponseQuery{}, fmt.Errorf("unexpected native path %s", path)
				}
			})
			s.reader = planningSolana(t, func(keys []solana.PublicKey) ([]*rpc.Account, error) {
				if failure == "status" || failure == "history-and-status" {
					return nil, errors.New("status observation unavailable")
				}
				return make([]*rpc.Account, len(keys)), nil
			})
			result, err := s.unresolved(context.Background(), native.Snapshot{Height: 100}, nil)
			after, readErr := os.ReadFile(s.store.discoveryPath)
			require.NoError(t, readErr)
			if failure == "status" || failure == "history-and-status" {
				require.Error(t, err)
				require.Equal(t, before, after)
				return
			}
			require.NoError(t, err)
			want := []sink.ID{values[4].ID, values[3].ID, values[5].ID, values[2].ID, values[6].ID, values[8].ID, values[7].ID}
			if failure == "" || failure == "history-after-page" {
				want = append(want, values[9].ID)
			}
			require.Equal(t, want, result.ids)
			require.Equal(t, []sink.ID{values[6].ID, values[8].ID, values[7].ID}, requeued)
			require.Equal(t, sink.HoldNone, result.known[values[4].ID].Settlement.Hold)
			if failure != "" {
				require.Equal(t, before, after)
				require.Contains(t, events.String(), "relay-discovery-failed")
			} else {
				require.NotEqual(t, before, after)
				saved, err := s.store.loadDiscovery()
				require.NoError(t, err)
				require.Equal(t, values[9].ID, saved.LastID)
				require.Equal(t, checkpoint.Key, saved.Key)
			}
		})
	}
}

func TestDiscoveryReadsCheckpointEachTick(t *testing.T) {
	s := planningService(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(s.store.discoveryPath), 0o700))
	require.NoError(t, os.WriteFile(s.store.discoveryPath, []byte(`{"version":2}`), 0o600))
	_, err := s.unresolved(context.Background(), native.Snapshot{Height: 100}, nil)
	require.Error(t, err)
}

func TestDiscoveryResolvedIdentityBeforeCheckpoint(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(strconv.FormatBool(mismatch), func(t *testing.T) {
			s := planningService(t)
			var fixture struct {
				Owner, Height, Sequence string
				Accounts                map[string]string
			}
			raw, err := os.ReadFile("bridge/testdata/wire.json")
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &fixture))
			transfer := planningTransfer(1)
			transfer.ID, err = native.ParseID(fixture.Owner + "/" + fixture.Height + "/" + fixture.Sequence)
			require.NoError(t, err)
			transfer.Status = sink.StatusMinted
			if mismatch {
				transfer.ID.Sequence++
			}
			s.native = planningNative(t, func(path string, height int64, data []byte, prove bool) (abci.ResponseQuery, error) {
				if path == "/akash.sink.v1.Query/Disputes" {
					return planningReply(t, &sink.QueryDisputesResponse{
						Pagination: &query.PageResponse{},
					}, height), nil
				}
				var request sink.QueryTransfersRequest
				require.NoError(t, request.Unmarshal(data))
				response := &sink.QueryTransfersResponse{
					Pagination: &query.PageResponse{},
				}
				if request.Status == sink.StatusInvalid {
					response.Transfers = []sink.TransferState{transfer}
				}
				return planningReply(t, response, height), nil
			})
			s.reader = planningSolana(t, func(keys []solana.PublicKey) ([]*rpc.Account, error) {
				require.Len(t, keys, 1)
				return []*rpc.Account{
					{
						RentEpoch: new(big.Int),
						Owner:     solana.PublicKey(bytes.Repeat([]byte{1}, 32)),
						Lamports:  1,
						Data:      rpc.DataBytesOrJSONFromBytes(planningHex(t, fixture.Accounts["TransferStatus"])),
					},
				}, nil
			})
			result, err := s.unresolved(context.Background(), native.Snapshot{Height: 10001}, nil)
			if mismatch {
				require.ErrorContains(t, err, "identity mismatch")
				_, err = os.Stat(s.store.discoveryPath)
				require.ErrorIs(t, err, os.ErrNotExist)
			} else {
				require.NoError(t, err)
				require.Empty(t, result.ids)
				require.Equal(t, transfer, result.known[transfer.ID])
				cursor, err := s.store.loadDiscovery()
				require.NoError(t, err)
				require.Equal(t, transfer.ID, cursor.LastID)
			}
		})
	}
}
