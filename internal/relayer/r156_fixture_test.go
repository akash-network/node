package relayer

// The R156 interpreter derives its RPC envelope and durable-send assertions from
// headerjob_test.go. It models allocation and owned cleanup, not SBF execution.
import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

const r156MockRent = 123
const r156BatchSize = 8 + 1 + 32 + 32 + 1 + 4 + 128*32
const r156ChunkSize = 8 + 32 + 4 + 900

type r156RPC struct {
	*headerJobRPC
	beforeSend func(*solana.Transaction, string)
	afterApply func(*solana.Transaction, string)
	topups     map[solana.PublicKey]uint64
	refunds    map[solana.PublicKey]uint64
}

func newR156Fixture(t *testing.T) *r156RPC {
	t.Helper()
	h := &r156RPC{
		headerJobRPC: newHeaderJobFixture(t),
		topups:       make(map[solana.PublicKey]uint64),
		refunds:      make(map[solana.PublicKey]uint64),
	}
	server := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(server.Close)
	client := rpc.New(server.URL)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	h.s.solana = client
	h.s.reader = bridge.NewReader(client, h.s.cfg.program, h.s.cfg.chainID, nil)
	return h
}

// Explicit System ownership and empty binary data follow System allocate;
// lamports only determine the mock rent top-up, never eligibility.
func r156CanAllocate(a *rpc.Account) bool {
	return a == nil || (!a.Executable && a.Owner == solana.SystemProgramID && a.Data != nil && a.Data.GetBinary() != nil && len(a.Data.GetBinary()) == 0)
}

func r156BatchBytes(payer solana.PublicKey, session [32]byte, bump byte, hashes [][32]byte) []byte {
	data := append(headerJobDisc("account", "SignatureBatch"), 1)
	data = append(data, payer[:]...)
	data = append(data, session[:]...)
	data = append(data, bump)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(hashes)))
	for _, hash := range hashes {
		data = append(data, hash[:]...)
	}
	return append(data, make([]byte, r156BatchSize-len(data))...)
}

func (h *r156RPC) validBatch(address solana.PublicKey, a *rpc.Account) bool {
	if a == nil || a.Executable || a.Owner != h.f.Program || a.Data == nil || len(a.Data.GetBinary()) != r156BatchSize {
		return false
	}
	batch, err := bridge.DecodeBatch(a.Data.GetBinary())
	if err != nil || batch.Version != 1 || batch.Submitter != h.s.payer.PublicKey() || len(batch.Hashes) > signatureBatchCapacity {
		return false
	}
	derived, err := solana.CreateProgramAddress([][]byte{[]byte("sig_batch"), batch.Submitter[:], batch.Session[:], {batch.Bump}}, h.f.Program)
	if err != nil || derived != address {
		return false
	}
	for i := 1; i < len(batch.Hashes); i++ {
		if bytes.Compare(batch.Hashes[i-1][:], batch.Hashes[i][:]) >= 0 {
			return false
		}
	}
	return true
}

func (h *r156RPC) allocate(address solana.PublicKey, data []byte) {
	balance := uint64(0)
	if a := h.accounts[address]; a != nil {
		balance = a.Lamports
	}
	if balance < r156MockRent {
		h.topups[address] += r156MockRent - balance
		balance = r156MockRent
	}
	a := headerJobAccount(h.f.Program, data)
	a.Lamports = balance
	h.accounts[address] = a
}

func (h *r156RPC) apply(tx *solana.Transaction, stage string) any {
	if stage != "chunk" && stage != "signature-batch-create" && stage != "signature" && stage != "cleanup" {
		h.headerJobRPC.apply(tx, stage)
		return nil
	}
	payer := h.s.payer.PublicKey()
	for _, ix := range tx.Message.Instructions {
		if tx.Message.AccountKeys[ix.ProgramIDIndex] != h.f.Program {
			continue
		}
		key := func(i int) solana.PublicKey { return tx.Message.AccountKeys[ix.Accounts[i]] }
		switch stage {
		case "signature-batch-create":
			session := [32]byte(ix.Data[8:40])
			address, bump, err := solana.FindProgramAddress([][]byte{[]byte("sig_batch"), payer[:], session[:]}, h.f.Program)
			require.NoError(h.t, err)
			if key(0) != address || key(3) != payer || h.access == bridge.AccessNone || !r156CanAllocate(h.accounts[address]) {
				return "mock batch initialization refused"
			}
			h.allocate(address, r156BatchBytes(payer, session, bump, nil))
		case "signature":
			a := h.accounts[key(0)]
			if !h.validBatch(key(0), a) || key(3) != payer || h.access == bridge.AccessNone {
				return "mock preverification identity refused"
			}
			batch, err := bridge.DecodeBatch(a.Data.GetBinary())
			require.NoError(h.t, err)
			hash := [32]byte(ix.Data[8:40])
			found := false
			for _, saved := range batch.Hashes {
				found = found || saved == hash
			}
			if !found {
				if len(batch.Hashes) == signatureBatchCapacity {
					return "mock batch capacity exceeded"
				}
				batch.Hashes = append(batch.Hashes, hash)
				sort.Slice(batch.Hashes, func(i, j int) bool { return bytes.Compare(batch.Hashes[i][:], batch.Hashes[j][:]) < 0 })
			}
			a.Data = rpc.DataBytesOrJSONFromBytes(r156BatchBytes(payer, batch.Session, batch.Bump, batch.Hashes))
		case "chunk":
			target := binary.LittleEndian.Uint64(ix.Data[8:16])
			address, err := h.f.ChunkAt(payer, target, ix.Data[16])
			require.NoError(h.t, err)
			if key(0) != address || key(4) != payer || h.access == bridge.AccessNone {
				return "mock chunk authority refused"
			}
			a := h.accounts[address]
			if !r156CanAllocate(a) {
				if a.Executable || a.Owner != h.f.Program || len(a.Data.GetBinary()) != r156ChunkSize || a.Lamports < r156MockRent {
					return "mock chunk initialization refused"
				}
				if _, err := bridge.DecodeChunk(a.Data.GetBinary()); err != nil {
					return "mock chunk decode refused"
				}
			}
			data := append(headerJobDisc("account", "HeaderChunk"), payer[:]...)
			data = append(data, ix.Data[17:]...)
			if len(data) > r156ChunkSize {
				return "mock chunk capacity exceeded"
			}
			h.allocate(address, append(data, make([]byte, r156ChunkSize-len(data))...))
		case "cleanup":
			if key(0) != payer {
				return "mock cleanup submitter refused"
			}
			for i := 1; i < len(ix.Accounts); i++ {
				address := key(i)
				a := h.accounts[address]
				if a == nil || a.Executable || a.Owner != h.f.Program || a.Lamports == 0 || a.Data == nil {
					continue
				}
				chunk, err := bridge.DecodeChunk(a.Data.GetBinary())
				ownedChunk := err == nil && chunk.Submitter == payer
				if ownedChunk || h.validBatch(address, a) {
					h.refunds[address] += a.Lamports
					delete(h.accounts, address)
				}
			}
		}
	}
	return nil
}

func (h *r156RPC) serve(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var call headerJobCall
	if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
		h.t.Error(err)
		w.WriteHeader(400)
		return
	}
	h.calls = append(h.calls, call)
	if call.Method == h.failMethod {
		http.Error(w, "injected RPC unavailable", 503)
		return
	}
	var result any
	switch call.Method {
	case "getGenesisHash":
		result = h.genesis
	case "getBalance":
		result = map[string]any{
			"context": map[string]any{
				"slot": h.slot,
			},
			"value": 10_000_000,
		}
	case "getSlot":
		if !h.warmStuck {
			h.slot++
		}
		result = h.slot
	case "getAccountInfo":
		var address solana.PublicKey
		require.NoError(h.t, json.Unmarshal(call.Params[0], &address))
		if address == h.failAddress {
			http.Error(w, "injected account unavailable", 503)
			return
		}
		result = map[string]any{
			"context": map[string]any{
				"slot": h.slot,
			},
			"value": h.accounts[address],
		}
	case "getMultipleAccounts":
		var addresses []solana.PublicKey
		require.NoError(h.t, json.Unmarshal(call.Params[0], &addresses))
		accounts := make([]*rpc.Account, len(addresses))
		for i, address := range addresses {
			accounts[i] = h.accounts[address]
		}
		result = map[string]any{
			"context": map[string]any{
				"slot": h.slot,
			},
			"value": accounts,
		}
	case "getProgramAccounts":
		accounts := []*rpc.KeyedAccount{}
		for address, account := range h.accounts {
			data := account.Data.GetBinary()
			if account.Owner == h.f.Program && len(data) == 88 && bytes.Equal(data[:8], headerJobDisc("account", "ConsensusStateStore")) {
				accounts = append(accounts, &rpc.KeyedAccount{
					Pubkey:  address,
					Account: account,
				})
			}
		}
		result = accounts
	case "getLatestBlockhash":
		result = map[string]any{
			"context": map[string]any{
				"slot": h.slot,
			},
			"value": map[string]any{
				"blockhash":            solana.Hash(sha256.Sum256([]byte(fmt.Sprint(len(h.calls))))).String(),
				"lastValidBlockHeight": 1000,
			},
		}
	case "getBlockHeight":
		result = 100
		if h.blockHeight != 0 {
			result = h.blockHeight
		}
	case "sendTransaction":
		var encoded string
		require.NoError(h.t, json.Unmarshal(call.Params[0], &encoded))
		raw, err := base64.StdEncoding.DecodeString(encoded)
		require.NoError(h.t, err)
		tx, err := solana.TransactionFromBytes(raw)
		require.NoError(h.t, err)
		require.NoError(h.t, tx.VerifySignatures())
		require.LessOrEqual(h.t, len(raw), 1232)
		require.Equal(h.t, solana.MessageVersionV0, tx.Message.GetVersion())
		require.NoError(h.t, tx.Message.SetAddressTables(h.tables))
		require.NoError(h.t, tx.Message.ResolveLookups())
		stage := h.stage(tx)
		if h.beforeSend != nil {
			h.beforeSend(tx, stage)
		}
		if stage != "bootstrap" && stage != "consensus-prune" {
			ids, err := h.s.store.sessionIDs()
			require.NoError(h.t, err)
			found := false
			for _, id := range ids {
				path, err := h.s.store.sessionPendingPath(id)
				if stage == "lookup-close" {
					path, err = h.s.store.sessionClosePendingPath(id)
				}
				require.NoError(h.t, err)
				pending, err := h.s.store.loadIntent(path)
				require.NoError(h.t, err)
				if pending != nil && pending.Raw == encoded {
					found = true
				}
			}
			require.True(h.t, found, "signed bytes must be durable before send")
		}
		h.broadcasts = append(h.broadcasts, encoded)
		if stage == h.dropStage {
			http.Error(w, "request lost before landing", 503)
			return
		}
		signature := tx.Signatures[0].String()
		if _, exists := h.transactions[signature]; !exists {
			entry := headerJobTransaction{
				raw:   encoded,
				stage: stage,
				logs:  []string{},
			}
			if stage == h.failStage {
				entry.failure = h.failure
				entry.logs = h.logs
				if stage == "assembly" && h.concurrent {
					require.Nil(h.t, h.apply(tx, stage))
				}
			} else {
				entry.failure = h.apply(tx, stage)
			}
			h.transactions[signature] = entry
			if h.afterApply != nil {
				h.afterApply(tx, stage)
			}
		}
		h.stages = append(h.stages, stage)
		if stage == h.lostStage {
			http.Error(w, "response lost after landing", 503)
			return
		}
		result = signature
	case "getSignatureStatuses":
		var signatures []string
		require.NoError(h.t, json.Unmarshal(call.Params[0], &signatures))
		var status any
		if entry, ok := h.transactions[signatures[0]]; ok {
			status = map[string]any{
				"slot":               h.slot,
				"confirmations":      nil,
				"err":                entry.failure,
				"confirmationStatus": "finalized",
			}
		}
		result = map[string]any{
			"context": map[string]any{
				"slot": h.slot,
			},
			"value": []any{status},
		}
	case "getTransaction":
		var signature string
		require.NoError(h.t, json.Unmarshal(call.Params[0], &signature))
		entry := h.transactions[signature]
		if !h.metadataMissing {
			result = map[string]any{
				"slot":    h.slot,
				"version": 0,
				"meta": map[string]any{
					"err":                  entry.failure,
					"fee":                  5000,
					"computeUnitsConsumed": 1234,
					"preBalances":          []uint64{10000000},
					"postBalances":         []uint64{9995000},
					"logMessages":          entry.logs,
				},
			}
		}
	default:
		h.t.Errorf("unexpected RPC method: %s", call.Method)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("content-type", "application/json")
	require.NoError(h.t, json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      call.ID,
		"result":  result,
	}))
}
