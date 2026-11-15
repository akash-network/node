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
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
)

type headerJobCall struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

type headerJobTransaction struct {
	raw     string
	stage   string
	failure any
	logs    []string
}

type headerJobRPC struct {
	mu              sync.Mutex
	t               *testing.T
	s               *service
	f               bridge.Followed
	packet          header.Packet
	accounts        map[solana.PublicKey]*rpc.Account
	tables          map[solana.PublicKey]solana.PublicKeySlice
	transactions    map[string]headerJobTransaction
	calls           []headerJobCall
	changeResult    func(headerJobCall, any) any
	stages          []string
	pruned          []uint64
	now             time.Time
	slot            uint64
	clock           int64
	latest          uint64
	trusting        uint64
	access          bridge.Access
	failMethod      string
	failAddress     solana.PublicKey
	failStage       string
	lostStage       string
	dropStage       string
	broadcasts      []string
	blockHeight     uint64
	metadataMissing bool
	concurrent      bool
	failure         any
	logs            []string
	warmStuck       bool
	genesis         string
	out             bytes.Buffer
	errOut          bytes.Buffer
}

func headerJobDisc(namespace, name string) []byte {
	hash := sha256.Sum256([]byte(namespace + ":" + name))
	return append([]byte{}, hash[:8]...)
}

func headerJobString(data []byte, value string) []byte {
	data = binary.LittleEndian.AppendUint32(data, uint32(len(value)))
	return append(data, value...)
}

func headerJobAccount(owner solana.PublicKey, data []byte) *rpc.Account {
	return &rpc.Account{
		RentEpoch: new(big.Int),
		Owner:     owner,
		Lamports:  123,
		Data:      rpc.DataBytesOrJSONFromBytes(data),
	}
}

func (h *headerJobRPC) clientData() []byte {
	data := headerJobString(headerJobDisc("account", "ClientState"), h.s.cfg.chainID)
	for _, n := range []uint64{1, 3, h.trusting, 1814400, 10, 0, 0, 1, h.latest} {
		data = binary.LittleEndian.AppendUint64(data, n)
	}
	return data
}

func headerJobConsensus(height uint64, packet header.Packet) []byte {
	data := binary.LittleEndian.AppendUint64(headerJobDisc("account", "ConsensusStateStore"), height)
	data = binary.LittleEndian.AppendUint64(data, packet.Timestamp)
	data = append(data, packet.Root[:]...)
	return append(data, packet.NextValidatorsHash[:]...)
}

func (h *headerJobRPC) accessData() []byte {
	data := binary.LittleEndian.AppendUint32(headerJobDisc("account", "AccessManager"), 1)
	data = binary.LittleEndian.AppendUint64(data, 1)
	count := uint32(0)
	if h.access != bridge.AccessNone {
		count = 1
	}
	data = binary.LittleEndian.AppendUint32(data, count)
	if count == 1 {
		member := h.s.payer.PublicKey()
		if h.access == bridge.AccessOpen {
			member = h.f.Program
		}
		data = append(data, member[:]...)
	}
	data = binary.LittleEndian.AppendUint32(data, 1)
	data = append(data, h.f.Program[:]...)
	return binary.LittleEndian.AppendUint32(data, 0)
}

func (h *headerJobRPC) lookupData(address solana.PublicKey, deactivation, extended uint64, authority solana.PublicKey) []byte {
	data := binary.LittleEndian.AppendUint32(nil, 1)
	data = binary.LittleEndian.AppendUint64(data, deactivation)
	data = binary.LittleEndian.AppendUint64(data, extended)
	data = append(data, 0, 1)
	data = append(data, authority[:]...)
	data = append(data, 0, 0)
	for _, key := range h.tables[address] {
		data = append(data, key[:]...)
	}
	return data
}

func newHeaderJobFixture(t *testing.T) *headerJobRPC {
	t.Helper()
	directory := durableTestDirectory(t)

	seed := sha256.Sum256([]byte("LOCAL TEST relayer header lifecycle"))
	payer := solana.PrivateKey(ed25519.NewKeyFromSeed(seed[:]))
	genesis := solana.Hash(sha256.Sum256([]byte("LOCAL TEST header genesis"))).String()
	deployment, err := bridge.Derive(solana.MustPublicKeyFromBase58(developmentProgram))
	require.NoError(t, err)
	followed, err := bridge.DeriveClient(solana.MustPublicKeyFromBase58(localClientProgram), solana.MustPublicKeyFromBase58(localAccessProgram))
	require.NoError(t, err)
	h := &headerJobRPC{
		t:            t,
		f:            followed,
		accounts:     make(map[solana.PublicKey]*rpc.Account),
		tables:       make(map[solana.PublicKey]solana.PublicKeySlice),
		transactions: make(map[string]headerJobTransaction),
		now:          time.Unix(1000, 0),
		slot:         100,
		clock:        1000,
		latest:       10,
		trusting:     86400,
		access:       bridge.AccessMember,
		genesis:      genesis,
		packet: header.Packet{
			Header:    bytes.Repeat([]byte{17}, 1801),
			Timestamp: 999_000_000_000,
			Root:      sha256.Sum256([]byte("root")),
			Receipts: []header.Receipt{
				{
					Hash:    sha256.Sum256([]byte("receipt one")),
					Message: []byte{1},
				},
				{
					Hash:    sha256.Sum256([]byte("receipt two")),
					Message: []byte{2},
				},
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(server.Close)
	store, err := newStateStore(directory, "akash-scale-1", genesis, deployment.Program.String(), payer.PublicKey().String(), nil)
	require.NoError(t, err)
	client := rpc.New(server.URL)
	h.s = &service{
		cfg: config{
			chainID:              "akash-scale-1",
			program:              deployment.Program,
			stateDir:             directory,
			maxSignatures:        96,
			pruneIntervalSeconds: 60,
			local:                true,
		},
		deps: defaultDependencies(Streams{
			Out: &h.out,
			Err: &h.errOut,
		}),
		solana:     client,
		payer:      payer,
		genesis:    genesis,
		store:      store,
		reader:     bridge.NewReader(client, deployment.Program, "akash-scale-1", nil),
		deployment: deployment,
		alerts:     make(map[string]int64),
	}
	h.s.deps.now = func() time.Time {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.now
	}
	h.s.deps.sleep = func(ctx context.Context, delay time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		h.mu.Lock()
		h.now = h.now.Add(delay)
		h.mu.Unlock()
		return nil
	}
	app := append(headerJobDisc("account", "AppState"), followed.AccessManager[:]...)
	app = append(app, 0)
	app = append(app, make([]byte, 256)...)
	h.accounts[followed.AppState] = headerJobAccount(followed.Program, app)
	h.accounts[followed.ClientState] = headerJobAccount(followed.Program, h.clientData())
	h.accounts[followed.AccessState] = headerJobAccount(followed.AccessManager, h.accessData())
	bridgeData := append(headerJobDisc("account", "BridgeConfig"), followed.Program[:]...)
	bridgeData = headerJobString(bridgeData, h.s.cfg.chainID)
	for _, n := range []uint64{1_000_000_000_000, 10_000_000_000_000} {
		bridgeData = binary.LittleEndian.AppendUint64(bridgeData, n)
	}
	bridgeData = binary.LittleEndian.AppendUint32(bridgeData, 0)
	bridgeData = append(bridgeData, make([]byte, 17)...)
	h.accounts[deployment.Bridge] = headerJobAccount(deployment.Program, bridgeData)
	address, err := followed.ConsensusAt(h.latest)
	require.NoError(t, err)
	h.accounts[address] = headerJobAccount(followed.Program, headerJobConsensus(h.latest, h.packet))
	h.setClock()
	return h
}

func (h *headerJobRPC) setClock() {
	data := make([]byte, 32)
	data = binary.LittleEndian.AppendUint64(data, uint64(h.clock))
	h.accounts[solana.SysVarClockPubkey] = headerJobAccount(solana.SystemProgramID, data)
}

func (h *headerJobRPC) serve(w http.ResponseWriter, r *http.Request) {
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
			"context": map[string]any{"slot": h.slot},
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
					h.apply(tx, stage)
				}
			} else {
				h.apply(tx, stage)
			}
			h.transactions[signature] = entry
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
	if h.changeResult != nil {
		result = h.changeResult(call, result)
	}
	w.Header().Set("content-type", "application/json")
	require.NoError(h.t, json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      call.ID,
		"result":  result,
	}))
}

func (h *headerJobRPC) stage(tx *solana.Transaction) string {
	for _, ix := range tx.Message.Instructions {
		program := tx.Message.AccountKeys[ix.ProgramIDIndex]
		if program == solana.AddressLookupTableProgramID {
			return map[uint32]string{0: "lookup-create", 2: "lookup-extend", 3: "lookup-deactivate", 4: "lookup-close"}[binary.LittleEndian.Uint32(ix.Data)]
		}
		for name, stage := range map[string]string{
			"initialize_signature_batch": "signature-batch-create",
			"pre_verify_signature_batch": "signature",
			"upload_header_chunk":        "chunk",
			"assemble_and_update_client": "assembly",
			"cleanup_incomplete_upload":  "cleanup",
			"prune_consensus_state":      "consensus-prune",
		} {
			if len(ix.Data) >= 8 && bytes.Equal(ix.Data[:8], headerJobDisc("global", name)) {
				return stage
			}
		}
	}
	return "bootstrap"
}

func (h *headerJobRPC) apply(tx *solana.Transaction, stage string) {
	payer := h.s.payer.PublicKey()
	for _, ix := range tx.Message.Instructions {
		program := tx.Message.AccountKeys[ix.ProgramIDIndex]
		if program != h.f.Program && program != solana.AddressLookupTableProgramID && program != h.s.deployment.Program && program != h.f.AccessManager {
			continue
		}
		key := func(i int) solana.PublicKey { return tx.Message.AccountKeys[ix.Accounts[i]] }
		switch stage {
		case "lookup-create":
			h.tables[key(0)] = solana.PublicKeySlice{}
			h.accounts[key(0)] = headerJobAccount(program, h.lookupData(key(0), math.MaxUint64, h.slot, payer))
		case "lookup-extend":
			data := ix.Data[12:]
			for len(data) > 0 {
				h.tables[key(0)] = append(h.tables[key(0)], solana.PublicKeyFromBytes(data[:32]))
				data = data[32:]
			}
			h.accounts[key(0)] = headerJobAccount(program, h.lookupData(key(0), math.MaxUint64, h.slot, payer))
		case "lookup-deactivate":
			h.accounts[key(0)] = headerJobAccount(program, h.lookupData(key(0), h.slot, h.slot, payer))
		case "lookup-close":
			delete(h.accounts, key(0))
		case "signature-batch-create":
			data := append(headerJobDisc("account", "SignatureBatch"), 1)
			data = append(data, payer[:]...)
			data = append(data, ix.Data[8:40]...)
			data = append(data, 0)
			data = binary.LittleEndian.AppendUint32(data, 0)
			h.accounts[key(0)] = headerJobAccount(program, data)
		case "signature":
			batch, err := bridge.DecodeBatch(h.accounts[key(0)].Data.GetBinary())
			require.NoError(h.t, err)
			data := append(headerJobDisc("account", "SignatureBatch"), 1)
			data = append(data, payer[:]...)
			data = append(data, batch.Session[:]...)
			data = append(data, 0)
			data = binary.LittleEndian.AppendUint32(data, uint32(len(batch.Hashes)+1))
			for _, hash := range batch.Hashes {
				data = append(data, hash[:]...)
			}
			data = append(data, ix.Data[8:40]...)
			h.accounts[key(0)] = headerJobAccount(program, data)
		case "chunk":
			data := append(headerJobDisc("account", "HeaderChunk"), payer[:]...)
			data = append(data, ix.Data[17:]...)
			h.accounts[key(0)] = headerJobAccount(program, data)
		case "assembly":
			h.latest = binary.LittleEndian.Uint64(ix.Data[8:])
			h.accounts[h.f.ClientState] = headerJobAccount(program, h.clientData())
			h.accounts[key(4)] = headerJobAccount(program, headerJobConsensus(h.latest, h.packet))
		case "cleanup":
			for i := 1; i < len(ix.Accounts); i++ {
				account := h.accounts[key(i)]
				if account != nil && account.Owner == program && account.Lamports != 0 {
					delete(h.accounts, key(i))
				}
			}
		case "bootstrap":
			h.applyBootstrap(program, ix.Data, key)
		case "consensus-prune":
			h.pruned = append(h.pruned, binary.LittleEndian.Uint64(ix.Data[8:]))
			delete(h.accounts, key(3))
		}
	}
}

func (h *headerJobRPC) active(t *testing.T) *activeHeaderDoc {
	t.Helper()
	raw, err := json.Marshal(packetDocument(h.s.cfg.chainID, h.packet))
	require.NoError(t, err)
	job := &activeHeaderDoc{
		ChainID: h.s.cfg.chainID,
		Genesis: h.s.genesis,
		Client:  h.f.Program.String(),
		Target:  11,
		Trusted: 10,
		Packet:  raw,
	}
	require.NoError(t, h.s.store.saveActive(job))
	return job
}

func TestHeaderJobCompleteSplitUpload(t *testing.T) {
	h := newHeaderJobFixture(t)
	var result headerUploadResult
	locked, err := h.s.withPayer(context.Background(), func(p *payerSession) error {
		var err error
		result, err = p.uploadHeader(context.Background(), h.f, h.packet, 11, 10, 1)
		return err
	})
	require.True(t, locked)
	require.NoError(t, err)
	require.False(t, transactionFailed(result.assembly))
	require.Equal(t, 3, result.chunks)
	require.Equal(t, 2, result.preverified)
	require.Equal(t, uint64(5*123), result.temporaryRentReturned)
	require.Equal(t, []string{"lookup-create", "lookup-extend", "signature-batch-create", "signature", "signature-batch-create", "signature", "chunk", "chunk", "chunk", "assembly", "cleanup", "lookup-deactivate"}, h.stages)
	require.LessOrEqual(t, result.maxTransactionBytes, 1232)
	require.Equal(t, uint64(12*5000), result.fees)
	ids, err := h.s.store.sessionIDs()
	require.NoError(t, err)
	require.Len(t, ids, 1)
	doc, err := h.s.store.loadSession(ids[0])
	require.NoError(t, err)
	require.Equal(t, "assembly", doc.Assembly.Stage)
	path, err := h.s.store.sessionPendingPath(ids[0])
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	log, err := os.ReadFile(filepath.Join(h.s.store.headerDirectory, "transactions.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(log), `"event":"transaction"`)
	require.Contains(t, string(log), `"event":"submitted"`)
	for _, call := range h.calls {
		if call.Method == "getAccountInfo" && bytes.Contains(call.Params[1], []byte(`"finalized"`)) {
			require.NotContains(t, string(call.Params[1]), "minContextSlot")
		}
	}
}

func TestHeaderJobCrashReplayAndRetryPreserved(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.active(t)
	h.s.headerRetry = retryState{
		Failures: 3,
		RetryAt:  90000,
	}
	h.lostStage = "assembly"
	err := h.s.resumeHeader(context.Background(), nil)
	require.ErrorContains(t, err, "503")
	ids, err := h.s.store.sessionIDs()
	require.NoError(t, err)
	path, err := h.s.store.sessionPendingPath(ids[0])
	require.NoError(t, err)
	pending, err := h.s.store.loadIntent(path)
	require.NoError(t, err)
	require.Equal(t, "assembly", pending.Stage)
	original := pending.Raw
	require.NotContains(t, h.stages, "cleanup")
	h.lostStage = ""
	require.NoError(t, h.s.resumeHeader(context.Background(), nil))
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.Nil(t, active)
	require.Equal(t, int64(3), h.s.headerRetry.Failures)
	require.Equal(t, original, h.transactions[pending.Signature].raw)
	require.Equal(t, 1, countHeaderStage(h.stages, "assembly"))
	require.Contains(t, h.stages, "cleanup")
	require.Contains(t, h.out.String(), `"event":"authenticated-header"`)
}

func countHeaderStage(stages []string, wanted string) int {
	n := 0
	for _, stage := range stages {
		if stage == wanted {
			n++
		}
	}
	return n
}

func TestHeaderJobUnknownMetadataRetainsEverything(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.active(t)
	h.metadataMissing = true
	err := h.s.resumeHeader(context.Background(), nil)
	require.ErrorContains(t, err, "missing transaction metadata")
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.NotNil(t, active)
	require.NotContains(t, h.stages, "cleanup")
	ids, err := h.s.store.sessionIDs()
	require.NoError(t, err)
	path, err := h.s.store.sessionPendingPath(ids[0])
	require.NoError(t, err)
	pending, err := h.s.store.loadIntent(path)
	require.NoError(t, err)
	require.NotNil(t, pending)
	h.metadataMissing = false
	require.NoError(t, h.s.resumeHeader(context.Background(), nil))
}

func TestHeaderJobExhaustionAndRestartQuirk(t *testing.T) {
	for _, crash := range []bool{false, true} {
		t.Run(fmt.Sprint(crash), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.active(t)
			h.failStage = "assembly"
			h.failure = map[string]any{
				"InstructionError": []any{2, "ComputationalBudgetExceeded"},
			}
			h.logs = []string{"Program " + h.f.Program.String() + " failed: Computational budget exceeded"}
			if crash {
				h.failAddress = solana.PublicKey{}
				writes := 0
				h.s.store.writeJSON = func(path string, data []byte) error {
					if strings.HasSuffix(path, ".json") && bytes.Contains(data, []byte(`"stage":"assembly"`)) && bytes.Contains(data, []byte(`"assembly":`)) {
						writes++
						if writes == 1 {
							require.NoError(t, atomicJSON(path, data))
							return fmt.Errorf("crash after terminal persisted")
						}
					}
					return atomicJSON(path, data)
				}
				err := h.s.resumeHeader(context.Background(), nil)
				require.ErrorContains(t, err, "crash after terminal persisted")
				h.s.store.writeJSON = atomicJSON
				// Original raw intent re-observation retains logs. Remove only that test
				// intent to model a crash after successful observer and durable unlink.
				ids, err := h.s.store.sessionIDs()
				require.NoError(t, err)
				path, err := h.s.store.sessionPendingPath(ids[0])
				require.NoError(t, err)
				require.NoError(t, h.s.store.removeIntent(path))
			}
			err := h.s.resumeHeader(context.Background(), nil)
			if crash {
				require.ErrorContains(t, err, "UpdateClientFailed")
				require.NotErrorIs(t, err, header.ErrUnsupported)
			} else {
				require.ErrorIs(t, err, header.ErrUnsupported)
				require.EqualError(t, err, "assembly exhausted the compute budget")
			}
			active, err := h.s.store.loadActive()
			require.NoError(t, err)
			require.Nil(t, active)
			require.Contains(t, h.stages, "cleanup")
		})
	}
}

func TestHeaderJobLayoutBounds(t *testing.T) {
	h := newHeaderJobFixture(t)
	for _, test := range []struct {
		name     string
		bytes    int
		receipts int
		want     string
	}{
		{
			name:     "empty",
			bytes:    0,
			receipts: 1,
			want:     "1 to 255 chunks",
		},
		{
			name:     "chunks",
			bytes:    255*900 + 1,
			receipts: 1,
			want:     "1 to 255 chunks",
		},
		{
			name:     "receipts",
			bytes:    900,
			receipts: 257,
			want:     "at most 256",
		},
		{
			name:     "accounts",
			bytes:    60 * 900,
			receipts: 1,
			want:     "conservative limit is 64",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet := header.Packet{
				Header:   make([]byte, test.bytes),
				Receipts: make([]header.Receipt, test.receipts),
			}
			_, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), packet, 11, 10, 128)
			require.NoError(t, err)
			require.Contains(t, reason, test.want)
		})
	}
	layout, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), h.packet, 11, 10, 1)
	require.NoError(t, err)
	require.Empty(t, reason)
	require.Equal(t, layout.session, layout.sessions[0])
	require.NotEqual(t, layout.sessions[0], layout.sessions[1])
	raw, err := json.Marshal(packetDocument(h.s.cfg.chainID, h.packet))
	require.NoError(t, err)
	packet, err := runtimeHeaderPacket(raw, h.s.cfg.chainID)
	require.NoError(t, err)
	require.Equal(t, h.packet, packet)
}

func TestHeaderJobAbandonAndLocalGuard(t *testing.T) {
	h := newHeaderJobFixture(t)
	job := h.active(t)
	job.Client = solana.SystemProgramID.String()
	require.NoError(t, h.s.store.saveActive(job))
	require.NoError(t, h.s.resumeHeader(context.Background(), nil))
	require.Empty(t, h.stages)
	require.Contains(t, h.errOut.String(), `"event":"header-upload-abandoned"`)
	h.s.cfg.local = false
	require.ErrorContains(t, h.s.update(context.Background(), 20, true, nil), "only available")
	require.ErrorContains(t, h.s.bootstrap(context.Background()), "only the local")
}

func TestHeaderJobUnlandedReplayAndExpiry(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprint(expired), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.active(t)
			h.dropStage = "chunk"
			require.Error(t, h.s.resumeHeader(context.Background(), nil))
			ids, err := h.s.store.sessionIDs()
			require.NoError(t, err)
			path, err := h.s.store.sessionPendingPath(ids[0])
			require.NoError(t, err)
			pending, err := h.s.store.loadIntent(path)
			require.NoError(t, err)
			require.Equal(t, "chunk", pending.Stage)
			raw := pending.Raw
			broadcasts := len(h.broadcasts)
			h.dropStage = ""
			if expired {
				h.blockHeight = 1001
				require.ErrorContains(t, h.s.resumeHeader(context.Background(), nil), "expired transaction history unresolved")
				retained, err := h.s.store.loadIntent(path)
				require.NoError(t, err)
				require.Equal(t, raw, retained.Raw)
				require.Len(t, h.broadcasts, broadcasts)
				require.NotContains(t, h.stages, "cleanup")
			} else {
				require.NoError(t, h.s.resumeHeader(context.Background(), nil))
				require.Equal(t, raw, h.broadcasts[broadcasts])
				require.Equal(t, 3, countHeaderStage(h.stages, "chunk"))
			}
		})
	}
}

func (h *headerJobRPC) savedHeaderLookup(t *testing.T, slot int64) (*sessionDoc, solana.PublicKey) {
	t.Helper()
	layout, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), h.packet, 11, 10, signatureBatchCapacity)
	require.NoError(t, err)
	require.Empty(t, reason)
	_, address, err := bridge.CreateLookup(h.s.payer.PublicKey(), h.s.payer.PublicKey(), uint64(slot))
	require.NoError(t, err)
	text := address.String()
	doc := &sessionDoc{
		Version:    1,
		Genesis:    h.s.genesis,
		Payer:      h.s.payer.PublicKey().String(),
		Client:     h.f.Program.String(),
		Target:     11,
		Trusted:    10,
		Session:    hex.EncodeToString(layout.session[:]),
		Lookup:     &text,
		LookupSlot: &slot,
	}
	require.NoError(t, h.s.store.saveSession(doc.Session, doc))
	return doc, address
}

func TestHeaderJobLookupResumeFallbackAndMismatch(t *testing.T) {
	for _, mode := range []string{"old-slot", "wrong-address", "deactivated", "warm-timeout", "warm-canceled"} {
		t.Run(mode, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.active(t)
			doc, address := h.savedHeaderLookup(t, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "old-slot":
				h.slot = 1000
			case "wrong-address":
				h.tables[address] = solana.PublicKeySlice{h.f.Program}
				h.accounts[address] = headerJobAccount(solana.AddressLookupTableProgramID, h.lookupData(address, math.MaxUint64, 1, h.s.payer.PublicKey()))
			case "deactivated":
				h.tables[address] = solana.PublicKeySlice{}
				h.accounts[address] = headerJobAccount(solana.AddressLookupTableProgramID, h.lookupData(address, 20, 1, h.s.payer.PublicKey()))
			case "warm-timeout":
				h.warmStuck = true
			case "warm-canceled":
				h.warmStuck = true
				h.s.deps.sleep = func(ctx context.Context, delay time.Duration) error {
					if delay == 100*time.Millisecond {
						cancel()
					}
					return ctx.Err()
				}
			}
			err := h.s.resumeHeader(ctx, nil)
			if mode == "old-slot" {
				require.NoError(t, err)
				saved, err := h.s.store.loadSession(doc.Session)
				require.NoError(t, err)
				require.NotEqual(t, *doc.Lookup, *saved.Lookup)
				require.Greater(t, *saved.LookupSlot, int64(1000))
			} else {
				require.Error(t, err)
				require.NotContains(t, h.stages, "assembly")
				require.NotContains(t, h.stages, "cleanup")
				active, err := h.s.store.loadActive()
				require.NoError(t, err)
				require.NotNil(t, active)
			}
		})
	}
}

func TestHeaderJobExistingChunkGuardsAndReuse(t *testing.T) {
	for _, mode := range []string{"same", "different", "foreign", "outage"} {
		t.Run(mode, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.active(t)
			address, err := h.f.ChunkAt(h.s.payer.PublicKey(), 11, 0)
			require.NoError(t, err)
			payer := h.s.payer.PublicKey()
			data := append(headerJobDisc("account", "HeaderChunk"), payer[:]...)
			data = binary.LittleEndian.AppendUint32(data, 900)
			chunk := append([]byte{}, h.packet.Header[:900]...)
			if mode == "different" {
				chunk[0] ^= 1
			}
			data = append(data, chunk...)
			h.accounts[address] = headerJobAccount(h.f.Program, data)
			if mode == "foreign" {
				h.accounts[address].Owner = solana.SystemProgramID
			}
			if mode == "outage" {
				h.failAddress = address
			}
			err = h.s.resumeHeader(context.Background(), nil)
			if mode == "same" {
				require.NoError(t, err)
				require.Equal(t, 2, countHeaderStage(h.stages, "chunk"))
			} else {
				require.Error(t, err)
				require.Equal(t, 0, countHeaderStage(h.stages, "chunk"))
				require.NotContains(t, h.stages, "cleanup")
			}
		})
	}
}

func TestHeaderJobActiveAndStoredTargetGuards(t *testing.T) {
	for _, mode := range []string{"expired", "frozen", "wrong-root", "already"} {
		t.Run(mode, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.active(t)
			switch mode {
			case "expired":
				h.clock += int64(h.trusting)
				h.setClock()
			case "frozen":
				data := h.clientData()
				binary.LittleEndian.PutUint64(data[len(data)-24:], 1)
				h.accounts[h.f.ClientState] = headerJobAccount(h.f.Program, data)
			case "wrong-root", "already":
				h.latest = 11
				h.accounts[h.f.ClientState] = headerJobAccount(h.f.Program, h.clientData())
				packet := h.packet
				if mode == "wrong-root" {
					packet.Root[0] ^= 1
				}
				address, err := h.f.ConsensusAt(11)
				require.NoError(t, err)
				h.accounts[address] = headerJobAccount(h.f.Program, headerJobConsensus(11, packet))
			}
			err := h.s.resumeHeader(context.Background(), nil)
			if mode == "already" {
				require.NoError(t, err)
				require.Equal(t, []string{"cleanup"}, h.stages)
			} else {
				require.Error(t, err)
				require.Empty(t, h.stages)
			}
		})
	}
}

func TestHeaderJobTSLayoutOracle(t *testing.T) {
	// Captured by executing baseline uploadHeader through lookup extension with
	// test-owned RPC/payment boundaries. Baseline: 14b2cd7042adb1f901ec44e0013e55284bbbe131,
	// contracts/solana/local/header-upload.ts. Digests cover the ordered ALT keys.
	h := newHeaderJobFixture(t)
	for _, test := range []struct {
		perBatch int
		accounts int
		digest   string
	}{
		{
			perBatch: 128,
			accounts: 14,
			digest:   "9f3f4657599b13f403bbb2d9703112346202073649b70d1be07dc31f7a5f6b5d",
		},
		{
			perBatch: 1,
			accounts: 15,
			digest:   "3a691f0b1c27409509b8fbdeba3c3d7126f2f07b6e11cf5f817399230818db49",
		},
	} {
		layout, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), h.packet, 11, 10, test.perBatch)
		require.NoError(t, err)
		require.Empty(t, reason)
		require.Equal(t, "e30707affc9f056e777c2c6c7980be98336394d1d7fafce666cef86766493f37", hex.EncodeToString(layout.session[:]))
		hash := sha256.New()
		for _, address := range layout.addresses {
			hash.Write(address[:])
		}
		require.Len(t, layout.addresses, test.accounts)
		require.Equal(t, test.digest, hex.EncodeToString(hash.Sum(nil)))
		_, address, err := bridge.CreateLookup(h.s.payer.PublicKey(), h.s.payer.PublicKey(), 100)
		require.NoError(t, err)
		require.Equal(t, "AZftVxy3g9w5KKcoZc3cSwV3qqpPuysMKiqkAKGo8ShH", address.String())
	}
}

func TestHeaderJobConcurrentAuthentication(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.active(t)
	h.failStage = "assembly"
	h.failure = map[string]any{
		"InstructionError": []any{2, "InvalidArgument"},
	}
	h.concurrent = true
	require.NoError(t, h.s.resumeHeader(context.Background(), nil))
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.Nil(t, active)
	data, err := os.ReadFile(filepath.Join(h.s.store.headerDirectory, "transactions.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"event":"authenticated-concurrently"`)
	require.Contains(t, h.stages, "cleanup")
	require.Contains(t, h.out.String(), `"event":"authenticated-header"`)
}

func TestHeaderJobUnsupportedDropsActiveWithoutSpending(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.packet.Header = make([]byte, 60*900)
	h.active(t)
	_, reason, err := layoutHeader(h.f, h.s.payer.PublicKey(), h.packet, 11, 10, signatureBatchCapacity)
	require.NoError(t, err)
	require.NotEmpty(t, reason)

	err = h.s.resumeHeader(context.Background(), nil)
	require.ErrorIs(t, err, header.ErrUnsupported)
	require.EqualError(t, err, reason)
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.Nil(t, active)
	require.Empty(t, h.stages)
	ids, err := h.s.store.sessionIDs()
	require.NoError(t, err)
	require.Empty(t, ids)
	data, err := os.ReadFile(filepath.Join(h.s.store.headerDirectory, "transactions.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"event":"unsupported-header"`)
}

func TestHeaderJobMalformedAndMismatchedSessions(t *testing.T) {
	for _, mode := range []string{"identity", "terminal-error", "empty-header", "empty-message"} {
		t.Run(mode, func(t *testing.T) {
			h := newHeaderJobFixture(t)
			job := h.active(t)
			doc, _ := h.savedHeaderLookup(t, 1)
			packet, err := decodeActivePacket(job.Packet, h.s.cfg.chainID)
			require.NoError(t, err)
			switch mode {
			case "identity":
				doc.Target = 12
			case "terminal-error":
				doc.Assembly = &transactionDoc{
					Stage:     "assembly",
					Signature: "previous",
					Error:     json.RawMessage(`"invalid scalar"`),
				}
			case "empty-header":
				packet.Header = ""
			case "empty-message":
				packet.Signatures[0].Message = ""
			}
			job.Packet, err = json.Marshal(packet)
			require.NoError(t, err)
			require.NoError(t, h.s.store.saveSession(doc.Session, doc))
			if mode == "empty-header" || mode == "empty-message" {
				require.Error(t, h.s.store.saveActive(job))
				raw, err := json.Marshal(job)
				require.NoError(t, err)
				require.NoError(t, atomicJSON(h.s.store.activePath, raw))
			} else {
				require.NoError(t, h.s.store.saveActive(job))
			}
			require.Error(t, h.s.resumeHeader(context.Background(), nil))
			require.Empty(t, h.stages)
			active, err := h.s.store.loadActive()
			require.NoError(t, err)
			require.NotNil(t, active)
		})
	}
}

func TestHeaderJobRepinPacketDecodeOrder(t *testing.T) {
	for _, packet := range []struct {
		name string
		raw  string
	}{
		{
			name: "malformed-object",
			raw:  `{"chainId":"wrong","futurePacket":{"retain":true}}`,
		},
		{
			name: "string",
			raw:  `"bad packet"`,
		},
		{
			name: "array",
			raw:  `[1,2]`,
		},
		{
			name: "null",
			raw:  `null`,
		},
		{
			name: "missing",
		},
	} {
		for _, client := range []string{"old-client", "", "same-client"} {
			t.Run(packet.name+"/"+client, func(t *testing.T) {
				h := newHeaderJobFixture(t)
				owner := client
				if client == "same-client" {
					owner = h.f.Program.String()
				}
				fields := map[string]any{
					"chainId": h.s.cfg.chainID,
					"genesis": h.s.genesis,
					"client":  owner,
					"target":  11,
					"trusted": 10,
					"future":  []any{"preserve", true},
				}
				if packet.raw != "" {
					fields["packet"] = json.RawMessage(packet.raw)
				}
				raw, err := json.MarshalIndent(fields, "", "   ")
				require.NoError(t, err)
				raw = append(raw, '\n')
				require.NoError(t, atomicJSON(h.s.store.activePath, raw))
				err = h.s.resumeHeader(context.Background(), nil)
				archives, globErr := filepath.Glob(filepath.Join(h.s.store.headerDirectory, "abandoned-*.json"))
				require.NoError(t, globErr)
				if client == "same-client" {
					require.Error(t, err)
					require.Empty(t, archives)
					current, err := os.ReadFile(h.s.store.activePath)
					require.NoError(t, err)
					require.Equal(t, raw, current)
				} else {
					require.NoError(t, err)
					require.Len(t, archives, 1)
					archived, err := os.ReadFile(archives[0])
					require.NoError(t, err)
					require.Equal(t, raw, archived)
					_, err = os.Stat(h.s.store.activePath)
					require.ErrorIs(t, err, os.ErrNotExist)
				}
				require.Empty(t, h.stages)
			})
		}
	}
}

func TestHeaderJobRepinInvalidIdentityNeverArchives(t *testing.T) {
	for _, test := range []struct {
		field string
		value any
	}{
		{
			field: "chainId",
			value: "another-chain",
		},
		{
			field: "genesis",
			value: "another-cluster",
		},
		{
			field: "client",
			value: nil,
		},
		{
			field: "client",
			value: 1,
		},
		{
			field: "target",
			value: 10,
		},
		{
			field: "target",
			value: maxSafeInteger + 1,
		},
		{
			field: "target",
			value: "11",
		},
		{
			field: "trusted",
			value: 0,
		},
		{
			field: "trusted",
			value: 11,
		},
	} {
		t.Run(fmt.Sprintf("%s/%v", test.field, test.value), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			fields := map[string]any{
				"chainId": h.s.cfg.chainID,
				"genesis": h.s.genesis,
				"client":  "old-client",
				"target":  11,
				"trusted": 10,
				"packet":  nil,
			}
			fields[test.field] = test.value
			raw, err := json.Marshal(fields)
			require.NoError(t, err)
			require.NoError(t, atomicJSON(h.s.store.activePath, raw))
			require.Error(t, h.s.resumeHeader(context.Background(), nil))
			archives, err := filepath.Glob(filepath.Join(h.s.store.headerDirectory, "abandoned-*.json"))
			require.NoError(t, err)
			require.Empty(t, archives)
			retained, err := os.ReadFile(h.s.store.activePath)
			require.NoError(t, err)
			require.Equal(t, raw, retained)
			require.Empty(t, h.stages)
		})
	}
}
