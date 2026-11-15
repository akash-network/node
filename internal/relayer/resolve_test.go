package relayer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmttypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"
	ics23 "github.com/cosmos/ics23/go"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type resolutionSent struct {
	raw      string
	stage    string
	delta    int64
	failure  any
	accounts []solana.PublicKey
	data     [][]byte
}

type resolutionHarness struct {
	mu             sync.Mutex
	t              *testing.T
	s              *service
	followed       bridge.Followed
	accounts       map[solana.PublicKey]*rpc.Account
	transfers      map[sink.ID]sink.TransferState
	proofs         map[sink.ID]native.Proof
	visible        []sink.ID
	sent           map[string]resolutionSent
	stages         []string
	statusIDs      []solana.PublicKey
	accountReads   map[solana.PublicKey][]string
	calls          map[string]int
	failMethod     string
	failAccount    solana.PublicKey
	failStage      string
	lostStage      string
	stuckStage     string
	badPrefix      bool
	noMetadata     bool
	simulationFail bool
	afterSend      func(string)
	currentRead    func(sink.ID, int) sink.TransferState
	prepareGate    func(sink.ID)
	currentReads   int
	now            time.Time
	blockhash      uint64
	out            bytes.Buffer
	errOut         bytes.Buffer
}

func resolutionDiscriminator(namespace, name string) []byte {
	digest := sha256.Sum256([]byte(namespace + ":" + name))
	return digest[:8]
}

func resolutionAccountData(t *testing.T, owner solana.PublicKey, name string, value any) *rpc.Account {
	t.Helper()
	body, err := bin.MarshalBorsh(value)
	require.NoError(t, err)
	return &rpc.Account{
		RentEpoch: new(big.Int),
		Owner:     owner,
		Lamports:  1000,
		Data:      rpc.DataBytesOrJSONFromBytes(append(resolutionDiscriminator("account", name), body...)),
	}
}

func newResolutionHarness(t *testing.T) *resolutionHarness {
	t.Helper()
	directory := durableTestDirectory(t)
	identity := durableIdentity(t)
	seed := sha256.Sum256([]byte("go-relayer-durable-test-payer-v1"))
	payer := solana.PrivateKey(ed25519.NewKeyFromSeed(seed[:]))
	program := solana.MustPublicKeyFromBase58(identity.ProgramID)
	store, err := newStateStore(directory, identity.ChainID, identity.Genesis, identity.ProgramID, identity.Payer, nil)
	require.NoError(t, err)
	deployment, err := bridge.Derive(program)
	require.NoError(t, err)
	followed, err := bridge.DeriveClient(solana.PublicKey{7}, solana.PublicKey{8})
	require.NoError(t, err)
	followed.ChainID = identity.ChainID
	h := &resolutionHarness{
		t:            t,
		followed:     followed,
		accounts:     make(map[solana.PublicKey]*rpc.Account),
		transfers:    make(map[sink.ID]sink.TransferState),
		proofs:       make(map[sink.ID]native.Proof),
		sent:         make(map[string]resolutionSent),
		accountReads: make(map[solana.PublicKey][]string),
		calls:        make(map[string]int),
		now:          time.Now().Truncate(time.Second),
	}
	solanaServer := httptest.NewServer(http.HandlerFunc(h.solanaRPC))
	t.Cleanup(solanaServer.Close)
	client := rpc.New(solanaServer.URL)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	h.s = &service{
		cfg: config{
			program:              program,
			chainID:              identity.ChainID,
			maxLamports:          1_000_000,
			retryMaxSeconds:      900,
			refreshFraction:      0.5,
			poll:                 3 * time.Second,
			pruneIntervalSeconds: 600,
		},
		store:      store,
		payer:      payer,
		genesis:    identity.Genesis,
		solana:     client,
		deployment: deployment,
		reader:     bridge.NewReader(client, program, identity.ChainID, nil),
		deps: dependencies{
			now: func() time.Time {
				h.mu.Lock()
				defer h.mu.Unlock()
				return h.now
			},
			sleep: func(ctx context.Context, duration time.Duration) error {
				h.mu.Lock()
				defer h.mu.Unlock()
				h.now = h.now.Add(duration)
				return ctx.Err()
			},
			streams: Streams{
				Out: &h.out,
				Err: &h.errOut,
			},
		},
	}
	h.accounts[followed.ClientState] = resolutionAccountData(t, followed.Program, "ClientState", bridge.ClientState{
		ChainID:               identity.ChainID,
		TrustLevelNumerator:   1,
		TrustLevelDenominator: 3,
		TrustingPeriod:        3600,
		LatestHeight: bridge.Height{
			RevisionNumber: 1,
			RevisionHeight: 20,
		},
	})
	h.accounts[followed.AppState] = resolutionAccountData(t, followed.Program, "AppState", struct {
		Manager solana.PublicKey
		Pending bool
		Padding [256]byte
	}{
		Manager: followed.AccessManager,
	})
	h.accounts[deployment.Bridge] = resolutionAccountData(t, program, "BridgeConfig", bridge.BridgeConfig{
		LightClientProgram: followed.Program,
		ChainID:            identity.ChainID,
		Policy: bridge.MintPolicy{
			PerTransferMax: 1_000_000,
			DailyMintCap:   10_000_000,
		},
	})
	h.accounts[deployment.Config] = resolutionAccountData(t, program, "LaunchConfig", bridge.LaunchConfig{BridgeEnabled: true})
	h.accounts[followed.AccessState] = resolutionAccountData(t, followed.AccessManager, "AccessManager", bridge.AccessManager{})
	consensusAddress, err := followed.ConsensusAt(20)
	require.NoError(t, err)
	h.accounts[consensusAddress] = resolutionAccountData(t, followed.Program, "ConsensusStateStore", struct {
		Height    uint64
		Timestamp uint64
		Root      [32]byte
		Next      [32]byte
	}{
		Height:    20,
		Timestamp: uint64(h.now.Unix()) * 1_000_000_000,
	})
	clock := make([]byte, 40)
	binary.LittleEndian.PutUint64(clock[32:], uint64(h.now.Unix()))
	h.accounts[solana.SysVarClockPubkey] = &rpc.Account{
		RentEpoch: new(big.Int),
		Data:      rpc.DataBytesOrJSONFromBytes(clock),
	}
	nativeServer := httptest.NewServer(http.HandlerFunc(h.nativeRPC))
	t.Cleanup(nativeServer.Close)
	h.s.native, err = native.NewWithHTTP(nativeServer.URL, identity.ChainID, time.Minute, &http.Client{Timeout: 2 * time.Second})
	require.NoError(t, err)
	return h
}

func (h *resolutionHarness) proof(transfer sink.TransferState, padding int) native.Proof {
	h.t.Helper()
	key, value, err := native.ProofValue(transfer, 19)
	require.NoError(h.t, err)
	merkle := commitment.MerkleProof{
		Proofs: []*ics23.CommitmentProof{
			{
				Proof: &ics23.CommitmentProof_Exist{
					Exist: &ics23.ExistenceProof{
						Key:   key,
						Value: value,
						Leaf: &ics23.LeafOp{
							Prefix: bytes.Repeat([]byte{3}, padding),
						},
					},
				},
			},
			{
				Proof: &ics23.CommitmentProof_Exist{
					Exist: &ics23.ExistenceProof{
						Key:   []byte("sink"),
						Value: []byte{1, 2, 3},
					},
				},
			},
		},
	}
	raw, err := merkle.Marshal()
	require.NoError(h.t, err)
	proof := native.Proof{
		Transfer: transfer,
		Height:   20,
		Key:      key,
		Value:    value,
		Merkle:   raw,
	}
	h.transfers[transfer.ID] = transfer
	h.proofs[transfer.ID] = proof
	return proof
}

func (h *resolutionHarness) job(proof native.Proof) *proofJob {
	h.t.Helper()
	job, err := newProofJob(h.s.store.identity, proof.Transfer.ID, h.followed.Program.String(), proof.Height, proof.Key, proof.Value, proof.Merkle)
	require.NoError(h.t, err)
	return job
}

func (h *resolutionHarness) buffer(address solana.PublicKey, account bridge.ProofBuffer) {
	h.t.Helper()
	info := resolutionAccountData(h.t, h.s.cfg.program, "ProofBuffer", struct {
		Submitter solana.PublicKey
		Nonce     [32]byte
		Expected  uint32
		Data      []byte
	}{
		Submitter: account.Submitter,
		Nonce:     account.Nonce,
		Expected:  account.ExpectedLen,
		Data:      account.Data,
	})
	raw := info.Data.GetBinary()
	raw = append(raw, make([]byte, max(0, account.Allocated-len(raw)))...)
	info.Data = rpc.DataBytesOrJSONFromBytes(raw)
	info.Lamports = account.Lamports
	h.accounts[address] = info
}

func (h *resolutionHarness) status(id sink.ID) {
	h.t.Helper()
	owner, err := sdk.AccAddressFromBech32(id.Owner)
	require.NoError(h.t, err)
	raw := resolutionDiscriminator("account", "TransferStatus")
	raw = binary.LittleEndian.AppendUint32(raw, uint32(len(owner)))
	raw = append(raw, owner...)
	for _, value := range []uint64{uint64(id.Height), id.Sequence, 20, 100} {
		raw = binary.LittleEndian.AppendUint64(raw, value)
	}
	raw = append(raw, byte(bridge.OutcomeVoided), byte(bridge.VoidGovernance))
	address, err := h.s.deployment.Status(id)
	require.NoError(h.t, err)
	h.accounts[address] = &rpc.Account{
		RentEpoch: new(big.Int),
		Owner:     h.s.cfg.program,
		Data:      rpc.DataBytesOrJSONFromBytes(raw),
	}
}

func (h *resolutionHarness) solanaRPC(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var request struct {
		ID     json.RawMessage
		Method string
		Params []json.RawMessage
	}
	require.NoError(h.t, json.NewDecoder(r.Body).Decode(&request))
	h.calls[request.Method]++
	if h.failMethod == request.Method {
		http.Error(w, "injected RPC failure", http.StatusServiceUnavailable)
		return
	}
	var result any
	switch request.Method {
	case "getGenesisHash":
		result = h.s.genesis
	case "getLatestBlockhash":
		h.blockhash++
		var hash solana.Hash
		binary.LittleEndian.PutUint64(hash[:], h.blockhash)
		result = map[string]any{
			"context": map[string]any{"slot": 100},
			"value": map[string]any{
				"blockhash":            hash.String(),
				"lastValidBlockHeight": 1000,
			},
		}
	case "getBlockHeight":
		result = 100
	case "getAccountInfo", "getMultipleAccounts":
		var addresses []solana.PublicKey
		if request.Method == "getAccountInfo" {
			var address solana.PublicKey
			require.NoError(h.t, json.Unmarshal(request.Params[0], &address))
			addresses = []solana.PublicKey{address}
		} else {
			require.NoError(h.t, json.Unmarshal(request.Params[0], &addresses))
		}
		var options struct {
			Commitment     string
			MinContextSlot *uint64
		}
		require.NoError(h.t, json.Unmarshal(request.Params[1], &options))
		values := make([]*rpc.Account, len(addresses))
		for i, address := range addresses {
			h.accountReads[address] = append(h.accountReads[address], options.Commitment)
			if address != h.followed.AppState {
				require.Nil(h.t, options.MinContextSlot)
			}
			if address == h.failAccount && h.failAccount != (solana.PublicKey{}) {
				http.Error(w, "injected account outage", http.StatusServiceUnavailable)
				return
			}
			values[i] = h.accounts[address]
			for id := range h.transfers {
				status, err := h.s.deployment.Status(id)
				require.NoError(h.t, err)
				if status == address {
					h.statusIDs = append(h.statusIDs, address)
				}
			}
		}
		var value any = values
		if request.Method == "getAccountInfo" {
			value = values[0]
		}
		result = map[string]any{
			"context": map[string]any{"slot": 100},
			"value":   value,
		}
	case "simulateTransaction":
		var failure any
		if h.simulationFail {
			failure = "simulation rejection"
		}
		result = map[string]any{
			"context": map[string]any{"slot": 100},
			"value": map[string]any{
				"err": failure,
			},
		}
	case "sendTransaction":
		var raw string
		require.NoError(h.t, json.Unmarshal(request.Params[0], &raw))
		wire, err := base64.StdEncoding.DecodeString(raw)
		require.NoError(h.t, err)
		require.LessOrEqual(h.t, len(wire), 1232)
		tx, err := solana.TransactionFromBytes(wire)
		require.NoError(h.t, err)
		require.NoError(h.t, tx.VerifySignatures())
		require.Equal(h.t, solana.MessageVersionV0, tx.Message.GetVersion())
		signature := tx.Signatures[0].String()
		if _, exists := h.sent[signature]; !exists {
			entry := h.apply(tx)
			entry.raw = raw
			h.sent[signature] = entry
			h.stages = append(h.stages, entry.stage)
			if h.afterSend != nil {
				h.afterSend(entry.stage)
			}
		}
		entry := h.sent[signature]
		var options map[string]any
		require.NoError(h.t, json.Unmarshal(request.Params[1], &options))
		if entry.stage == "bootstrap" {
			require.NotEqual(h.t, true, options["skipPreflight"])
			require.Equal(h.t, "confirmed", options["preflightCommitment"])
		} else {
			require.Equal(h.t, true, options["skipPreflight"])
			path := ""
			for id := range h.transfers {
				candidate, err := h.s.store.paymentPath(id)
				require.NoError(h.t, err)
				intent, err := h.s.store.loadIntent(candidate)
				require.NoError(h.t, err)
				if intent != nil && intent.Signature == signature {
					path = candidate
					require.Equal(h.t, raw, intent.Raw)
				}
			}
			require.NotEmpty(h.t, path, "signed intent must exist before broadcast")
		}
		if h.lostStage == entry.stage {
			http.Error(w, "response lost after applying transaction", http.StatusServiceUnavailable)
			return
		}
		result = signature
	case "getSignatureStatuses":
		var signatures []string
		require.NoError(h.t, json.Unmarshal(request.Params[0], &signatures))
		var value any
		if entry, exists := h.sent[signatures[0]]; exists {
			status := "finalized"
			if entry.stage == "bootstrap" {
				status = "confirmed"
			}
			value = map[string]any{
				"confirmationStatus": status,
				"err":                entry.failure,
			}
		}
		result = map[string]any{
			"context": map[string]any{"slot": 100},
			"value":   []any{value},
		}
	case "getTransaction":
		if h.noMetadata {
			break
		}
		var signature string
		require.NoError(h.t, json.Unmarshal(request.Params[0], &signature))
		entry := h.sent[signature]
		result = map[string]any{
			"slot": 100,
			"meta": map[string]any{
				"err":                  entry.failure,
				"fee":                  5,
				"computeUnitsConsumed": 23,
				"preBalances":          []int64{1000000},
				"postBalances":         []int64{1000000 - entry.delta},
			},
		}
	default:
		h.t.Errorf("unexpected Solana RPC %s", request.Method)
		http.Error(w, "unexpected RPC", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	require.NoError(h.t, json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result":  result,
	}))
}

func (h *resolutionHarness) apply(tx *solana.Transaction) resolutionSent {
	entry := resolutionSent{delta: 5}
	for _, ix := range tx.Message.Instructions {
		if tx.Message.AccountKeys[ix.ProgramIDIndex] != h.s.cfg.program {
			continue
		}
		accounts := make([]solana.PublicKey, len(ix.Accounts))
		for i, index := range ix.Accounts {
			accounts[i] = tx.Message.AccountKeys[index]
		}
		data := []byte(ix.Data)
		require.GreaterOrEqual(h.t, len(data), 8)
		name := ""
		for _, candidate := range []string{"create_proof_buffer", "grow_proof_buffer", "append_proof", "close_proof_buffer", "mint_transfer", "void_transfer"} {
			if bytes.Equal(data[:8], resolutionDiscriminator("global", candidate)) {
				name = candidate
			}
		}
		stages := map[string]string{
			"create_proof_buffer": "proof-create",
			"grow_proof_buffer":   "proof-grow",
			"append_proof":        "proof-append",
			"close_proof_buffer":  "proof-close",
			"mint_transfer":       "resolve",
			"void_transfer":       "resolve",
		}
		entry.stage = stages[name]
		entry.accounts = accounts
		entry.data = append(entry.data, bytes.Clone(data))
		if entry.stage == h.failStage {
			entry.failure = map[string]any{"InstructionError": []any{0, "InvalidArgument"}}
			continue
		}
		if entry.stage == h.stuckStage {
			continue
		}
		switch name {
		case "create_proof_buffer":
			h.buffer(accounts[1], bridge.ProofBuffer{
				Submitter:   accounts[0],
				Nonce:       [32]byte(data[8:40]),
				ExpectedLen: binary.LittleEndian.Uint32(data[40:]),
				Allocated:   100,
				Lamports:    1000,
			})
			entry.delta += 1000
		case "grow_proof_buffer":
			account, err := bridge.DecodeProofBuffer(h.accounts[accounts[1]].Data.GetBinary())
			require.NoError(h.t, err)
			account.Allocated += 1000
			account.Lamports = h.accounts[accounts[1]].Lamports + 100
			h.buffer(accounts[1], account)
			entry.delta += 100
		case "append_proof":
			account, err := bridge.DecodeProofBuffer(h.accounts[accounts[1]].Data.GetBinary())
			require.NoError(h.t, err)
			offset := binary.LittleEndian.Uint32(data[8:])
			length := binary.LittleEndian.Uint32(data[12:])
			require.Equal(h.t, len(account.Data), int(offset))
			require.LessOrEqual(h.t, length, uint32(450))
			account.Data = append(account.Data, data[16:]...)
			account.Lamports = h.accounts[accounts[1]].Lamports
			if h.badPrefix {
				account.Data[0] ^= 1
			}
			h.buffer(accounts[1], account)
		case "close_proof_buffer":
			entry.delta -= int64(h.accounts[accounts[1]].Lamports)
			delete(h.accounts, accounts[1])
		case "mint_transfer", "void_transfer":
			entry.delta += 200
		default:
			h.t.Fatalf("unexpected test instruction %x", data)
		}
	}
	if entry.stage == "" {
		entry.stage = "bootstrap"
	}
	return entry
}

func (h *resolutionHarness) nativeRPC(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID     json.RawMessage
		Method string
		Params map[string]json.RawMessage
	}
	require.NoError(h.t, json.NewDecoder(r.Body).Decode(&request))
	if h.prepareGate != nil && request.Method == "abci_query" && string(request.Params["path"]) == `"/akash.sink.v1.Query/Transfer"` && string(request.Params["height"]) == `"19"` {
		var raw string
		require.NoError(h.t, json.Unmarshal(request.Params["data"], &raw))
		data, err := hex.DecodeString(strings.TrimPrefix(raw, "0x"))
		require.NoError(h.t, err)
		var query sink.QueryTransferRequest
		require.NoError(h.t, query.Unmarshal(data))
		h.prepareGate(query.ID)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var result any
	switch request.Method {
	case "status":
		result = &ctypes.ResultStatus{
			NodeInfo: p2p.DefaultNodeInfo{
				Network: h.s.cfg.chainID,
			},
			SyncInfo: ctypes.SyncInfo{
				LatestBlockHeight: 30,
				LatestBlockTime:   time.Now(),
			},
		}
	case "abci_info":
		result = &ctypes.ResultABCIInfo{
			Response: abci.ResponseInfo{
				LastBlockHeight: 30,
			},
		}
	case "block":
		result = &ctypes.ResultBlock{
			Block: &cmttypes.Block{
				Header: cmttypes.Header{
					Height:  30,
					ChainID: h.s.cfg.chainID,
					Time:    time.Now(),
				},
			},
		}
	case "abci_query":
		var path, textHeight, raw string
		require.NoError(h.t, json.Unmarshal(request.Params["path"], &path))
		require.NoError(h.t, json.Unmarshal(request.Params["height"], &textHeight))
		require.NoError(h.t, json.Unmarshal(request.Params["data"], &raw))
		height, err := strconv.ParseInt(textHeight, 10, 64)
		require.NoError(h.t, err)
		data, err := hex.DecodeString(strings.TrimPrefix(raw, "0x"))
		require.NoError(h.t, err)
		h.calls[path]++
		var reply abci.ResponseQuery
		switch path {
		case "/akash.sink.v1.Query/State":
			reply = planningReply(h.t, &sink.QueryStateResponse{
				Epoch: 1,
				SendUsage: sink.EpochUsage{
					Epoch:  1,
					Amount: sdk.NewInt64Coin("uakt", 0),
				},
				RefundUsage: sink.EpochUsage{
					Epoch:  1,
					Amount: sdk.NewInt64Coin("uakt", 0),
				},
			}, height)
		case "/akash.sink.v1.Query/Params":
			reply = planningReply(h.t, &sink.QueryParamsResponse{}, height)
		case "/akash.sink.v1.Query/Reporters":
			reply = planningReply(h.t, &sink.QueryReportersResponse{
				Reporters: []sink.ReporterPolicy{planningTransfer(1).Settlement.Policy},
			}, height)
		case "/akash.sink.v1.Query/Transfer":
			var query sink.QueryTransferRequest
			require.NoError(h.t, query.Unmarshal(data))
			transfer := h.transfers[query.ID]
			if height == 30 {
				h.currentReads++
				if h.currentRead != nil {
					transfer = h.currentRead(query.ID, h.currentReads)
				}
			} else {
				transfer = h.proofs[query.ID].Transfer
			}
			reply = planningReply(h.t, &sink.QueryTransferResponse{Transfer: transfer}, height)
		case "/store/sink/key":
			for _, proof := range h.proofs {
				if !bytes.Equal(proof.Key, data) {
					continue
				}
				var merkle commitment.MerkleProof
				require.NoError(h.t, merkle.Unmarshal(proof.Merkle))
				ops := &cmtcrypto.ProofOps{}
				for i, level := range merkle.Proofs {
					raw, err := level.Marshal()
					require.NoError(h.t, err)
					ops.Ops = append(ops.Ops, cmtcrypto.ProofOp{
						Type: []string{"ics23:iavl", "ics23:simple"}[i],
						Data: raw,
					})
				}
				reply = abci.ResponseQuery{
					Height:   height,
					Key:      data,
					Value:    proof.Value,
					ProofOps: ops,
				}
			}
		case "/akash.sink.v1.Query/Disputes":
			reply = planningReply(h.t, &sink.QueryDisputesResponse{
				Pagination: &query.PageResponse{},
			}, height)
		case "/akash.sink.v1.Query/Transfers":
			var request sink.QueryTransfersRequest
			require.NoError(h.t, request.Unmarshal(data))
			var transfers []sink.TransferState
			for _, id := range h.visible {
				transfer := h.transfers[id]
				if request.Status == sink.StatusInvalid || request.Status == transfer.Status {
					transfers = append(transfers, transfer)
				}
			}
			reply = planningReply(h.t, &sink.QueryTransfersResponse{
				Transfers:  transfers,
				Pagination: &query.PageResponse{},
			}, height)
		default:
			h.t.Fatalf("unexpected native path %s", path)
		}
		result = &ctypes.ResultABCIQuery{Response: reply}
	default:
		h.t.Fatalf("unexpected native method %s", request.Method)
	}
	encoded, err := cmtjson.Marshal(result)
	require.NoError(h.t, err)
	w.Header().Set("Content-Type", "application/json")
	require.NoError(h.t, json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result":  json.RawMessage(encoded),
	}))
}

func TestResolutionFullMintVoidAndFinalGuards(t *testing.T) {
	for _, mode := range []string{"mint", "unprepared", "void", "changed-before", "changed-after", "resolved-after", "prepared-failure", "status-outage"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			transfer := planningTransfer(1)
			if mode == "void" {
				transfer.Status = sink.StatusRefundPending
				transfer.Settlement.VoidAuthorization = &sink.VoidAuthorization{
					RequestedHeight: 12,
					RequestedRound:  1,
				}
			}
			proof := h.proof(transfer, 1200)
			prepared := &preparedProof{proof: proof}
			if mode == "unprepared" {
				prepared = nil
			}
			if mode == "prepared-failure" {
				prepared.err = errors.New("prepared proof failed")
			}
			if strings.HasPrefix(mode, "changed-") {
				h.currentRead = func(id sink.ID, count int) sink.TransferState {
					value := h.transfers[id]
					if mode == "changed-before" || count >= 2 {
						value.Status = sink.StatusRefunded
					}
					return value
				}
			}
			if mode == "resolved-after" {
				h.afterSend = func(stage string) {
					if stage == "proof-append" {
						h.status(transfer.ID)
					}
				}
			}
			if mode == "status-outage" {
				h.failAccount, _ = h.s.deployment.Status(transfer.ID)
			}
			var result string
			err := runDurablePayer(t, h.s, func(p *payerSession) error {
				var err error
				result, err = p.relayWith(context.Background(), h.followed, transfer.ID, 20, prepared)
				return err
			})
			if mode == "prepared-failure" || mode == "status-outage" {
				require.Error(t, err)
				require.Empty(t, h.stages)
				return
			}
			require.NoError(t, err)
			job, err := h.s.store.loadProof(transfer.ID)
			require.NoError(t, err)
			require.Nil(t, job)
			if strings.HasPrefix(mode, "changed-") {
				require.Equal(t, "skipped", result)
				require.NotContains(t, h.stages, "resolve")
				if mode == "changed-before" {
					require.Empty(t, h.stages)
					require.Contains(t, h.errOut.String(), "relay-final-check")
				} else {
					require.Equal(t, "proof-close", h.stages[len(h.stages)-1])
				}
				return
			}
			if mode == "resolved-after" {
				require.Equal(t, "already-resolved", result)
				require.NotContains(t, h.stages, "resolve")
				return
			}
			require.Equal(t, "resolved", result)
			require.Equal(t, []string{"proof-create", "proof-grow", "proof-grow", "proof-append", "proof-append", "resolve", "proof-close"}, h.stages)
			ledger, err := h.s.store.readLedger()
			require.NoError(t, err)
			require.Equal(t, int64(235), ledger.spent(transfer.ID))
			require.Equal(t, int64(1), ledger.entries[transfer.ID].Attempts)
			require.Equal(t, int64(1200), *ledger.entries[transfer.ID].BufferFundedLamports)
			require.Equal(t, int64(1200), *ledger.entries[transfer.ID].BufferReclaimedLamports)
			require.True(t, *ledger.entries[transfer.ID].Completed)
			for _, sent := range h.sent {
				if sent.stage != "resolve" {
					continue
				}
				name := "mint_transfer"
				if mode == "void" {
					name = "void_transfer"
				}
				require.Equal(t, resolutionDiscriminator("global", name), sent.data[0][:8])
				latest, err := h.followed.ConsensusAt(20)
				require.NoError(t, err)
				require.Contains(t, sent.accounts, latest)
			}
			require.Contains(t, h.out.String(), "transfer-resolved")
		})
	}
}

func TestResolutionStatusBudgetAndLockPrecedePreparation(t *testing.T) {
	for _, mode := range []string{"resolved", "budget", "busy", "foreign", "empty"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			proof := h.proof(planningTransfer(1), 0)
			id := proof.Transfer.ID
			failure := errors.New("unusable prepared proof")
			switch mode {
			case "resolved":
				h.status(id)
			case "budget":
				_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
					return true, ledger.charge(id, h.s.cfg.maxLamports+1, "prior", 0)
				})
				require.NoError(t, err)
			case "foreign", "empty":
				address, err := h.s.deployment.Status(id)
				require.NoError(t, err)
				owner := h.s.cfg.program
				data := []byte{}
				if mode == "foreign" {
					owner = solana.SystemProgramID
					data = make([]byte, 20)
				}
				h.accounts[address] = &rpc.Account{
					RentEpoch: new(big.Int),
					Owner:     owner,
					Data:      rpc.DataBytesOrJSONFromBytes(data),
				}
			}
			if mode == "busy" {
				require.NoError(t, runDurablePayer(t, h.s, func(*payerSession) error {
					result, err := h.s.relay(context.Background(), id, 20)
					require.ErrorIs(t, err, errPayerBusy)
					require.Equal(t, "skipped", result)
					return nil
				}))
			} else {
				var result string
				err := runDurablePayer(t, h.s, func(p *payerSession) error {
					var err error
					result, err = p.relayWith(context.Background(), h.followed, id, 20, &preparedProof{err: failure})
					return err
				})
				if mode == "resolved" || mode == "budget" {
					require.NoError(t, err)
					expected := "skipped"
					if mode == "resolved" {
						expected = "already-resolved"
					}
					require.Equal(t, expected, result)
				} else {
					require.ErrorIs(t, err, failure)
				}
			}
			require.Empty(t, h.stages)
			require.Zero(t, h.calls["/akash.sink.v1.Query/State"])
			require.Zero(t, h.calls["/store/sink/key"])
		})
	}
}
