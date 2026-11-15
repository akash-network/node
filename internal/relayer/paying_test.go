package relayer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type durableRPC struct {
	mu              sync.Mutex
	t               *testing.T
	identity        durableFixtureIdentity
	path            string
	now             time.Time
	genesis         string
	genesisFlipAt   int
	calls           map[string]int
	broadcasts      []string
	sendTimes       []time.Time
	landed          bool
	lostResponse    bool
	failMethod      string
	simulationFails bool
	simulationError json.RawMessage
	missingMetadata bool
	nullMetadata    int
	missingBalances bool
	failure         json.RawMessage
	status          string
	statusDelay     int
	blockHeight     uint64
	delta           int64
}

func (h *durableRPC) serve(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var request struct {
		ID     json.RawMessage   `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.t.Error(err)
		http.Error(w, "bad test request", http.StatusBadRequest)
		return
	}
	h.calls[request.Method]++
	if h.failMethod == request.Method {
		http.Error(w, "test RPC unavailable", http.StatusServiceUnavailable)
		return
	}
	var result any
	switch request.Method {
	case "getGenesisHash":
		result = h.genesis
		if h.genesisFlipAt > 0 && h.calls[request.Method] >= h.genesisFlipAt {
			result = h.identity.ProgramID
		}
	case "getLatestBlockhash":
		result = map[string]any{
			"context": map[string]int{"slot": 10},
			"value": map[string]any{
				"blockhash":            h.identity.Blockhash,
				"lastValidBlockHeight": 1000,
			},
		}
	case "getBlockHeight":
		if len(request.Params) != 1 || !bytes.Contains(request.Params[0], []byte(`"finalized"`)) {
			h.t.Error("expiry must use finalized height")
		}
		result = h.blockHeight
	case "simulateTransaction":
		if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
			h.t.Error("intent exists before simulation")
		}
		if len(request.Params) < 2 || !bytes.Contains(request.Params[1], []byte(`"sigVerify":true`)) || !bytes.Contains(request.Params[1], []byte(`"confirmed"`)) {
			h.t.Error("wrong simulation contract")
		}
		var failure any
		if h.simulationFails {
			failure = map[string]any{"InstructionError": []any{0, "InvalidArgument"}}
		}
		if h.simulationError != nil {
			failure = h.simulationError
		}
		result = map[string]any{
			"context": map[string]int{"slot": 10},
			"value": map[string]any{
				"err":  failure,
				"logs": []string{},
			},
		}
	case "sendTransaction":
		var encoded string
		if err := json.Unmarshal(request.Params[0], &encoded); err != nil {
			h.t.Error(err)
		}
		if h.path != "" {
			data, err := os.ReadFile(h.path)
			var doc intent
			if err != nil || json.Unmarshal(data, &doc) != nil || doc.Raw != encoded {
				h.t.Error("exact signed intent must be durable before broadcast")
			}
		}
		if len(request.Params) < 2 || !bytes.Contains(request.Params[1], []byte(`"maxRetries":0`)) || !bytes.Contains(request.Params[1], []byte(`"skipPreflight":true`)) {
			h.t.Error("resend changed retry/preflight options")
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			h.t.Error(err)
		}
		tx, err := solana.TransactionFromBytes(raw)
		if err != nil || tx.Message.GetVersion() != solana.MessageVersionV0 {
			h.t.Error("paid transaction is not signed v0")
		}
		if err := tx.VerifySignatures(); err != nil {
			h.t.Error(err)
		}
		h.broadcasts = append(h.broadcasts, encoded)
		h.sendTimes = append(h.sendTimes, h.now)
		h.landed = true
		result = tx.Signatures[0].String()
		if h.lostResponse {
			http.Error(w, "test response lost after land", http.StatusServiceUnavailable)
			return
		}
	case "getSignatureStatuses":
		if len(request.Params) < 2 || !bytes.Contains(request.Params[1], []byte(`"searchTransactionHistory":true`)) {
			h.t.Error("signature status must search history")
		}
		var status any
		if h.landed && h.calls[request.Method] > h.statusDelay {
			status = map[string]any{
				"slot":               20,
				"confirmations":      nil,
				"err":                h.failure,
				"confirmationStatus": h.status,
			}
		}
		result = map[string]any{
			"context": map[string]int{"slot": 20},
			"value":   []any{status},
		}
	case "getTransaction":
		if !h.landed && h.blockHeight > 1000 {
			if len(request.Params) != 2 || !bytes.Contains(request.Params[1], []byte(`"finalized"`)) || !bytes.Contains(request.Params[1], []byte(`"base64"`)) {
				h.t.Error("expired history lookup must use finalized base64")
			}
			break
		}
		if len(request.Params) < 2 || !bytes.Contains(request.Params[1], []byte(`"confirmed"`)) || !bytes.Contains(request.Params[1], []byte(`"maxSupportedTransactionVersion":0`)) {
			h.t.Error("wrong metadata commitment/version")
		}
		if h.calls[request.Method] <= h.nullMetadata {
			break
		}
		var meta any
		if !h.missingMetadata {
			balances := []int64{10000000}
			post := []int64{10000000 - h.delta}
			if h.missingBalances {
				balances = nil
				post = nil
			}
			meta = map[string]any{
				"err":                  h.failure,
				"fee":                  5000,
				"computeUnitsConsumed": 123,
				"preBalances":          balances,
				"postBalances":         post,
				"logMessages":          []string{"test execution"},
			}
		}
		result = map[string]any{
			"slot":    20,
			"version": 0,
			"meta":    meta,
		}
	default:
		h.t.Errorf("unexpected RPC method %s", request.Method)
		http.Error(w, "unexpected method", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result":  result,
	}); err != nil {
		h.t.Error(err)
	}
}

func durablePaymentService(t *testing.T) (*service, *durableRPC, txRequest) {
	t.Helper()
	identity := durableIdentity(t)
	store := durableStore(t)
	id, _ := native.ParseID(identity.ID)
	path, _ := store.paymentPath(id)
	seed := sha256.Sum256([]byte("go-relayer-durable-test-payer-v1"))
	payer := solana.PrivateKey(ed25519.NewKeyFromSeed(seed[:]))
	if payer.PublicKey().String() != identity.Payer {
		t.Fatal("Go and TS test key differ")
	}
	h := &durableRPC{
		t:           t,
		identity:    identity,
		path:        path,
		now:         time.Unix(1700000000, 0),
		genesis:     identity.Genesis,
		calls:       make(map[string]int),
		failure:     json.RawMessage(`null`),
		status:      "finalized",
		blockHeight: 10,
		delta:       5000,
	}
	server := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(server.Close)
	svc := &service{
		store:   store,
		solana:  rpc.New(server.URL),
		payer:   payer,
		genesis: identity.Genesis,
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
		},
	}
	recipient, _ := solana.PublicKeyFromBase58(identity.ProgramID)
	request := txRequest{
		stage:        "resolve",
		instructions: []solana.Instruction{system.NewTransferInstruction(1, payer.PublicKey(), recipient).Build()},
		preflight:    true,
	}
	return svc, h, request
}

func runDurablePayer(t *testing.T, service *service, action func(*payerSession) error) error {
	t.Helper()
	acquired, err := service.withPayer(context.Background(), action)
	if !acquired {
		t.Fatalf("test payer lock not acquired: %v", err)
	}
	return err
}

func TestDurablePaymentObserveBeforeUnlinkAndChargeOnce(t *testing.T) {
	for _, failure := range []string{`null`, `{"InstructionError":[0,"InvalidArgument"]}`} {
		t.Run(failure, func(t *testing.T) {
			svc, h, request := durablePaymentService(t)
			h.failure = json.RawMessage(failure)
			id, _ := native.ParseID(h.identity.ID)
			crash := errors.New("test crash after durable accounting")
			err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.send(context.Background(), request, h.path, func(result txResult) error {
					if err := p.chargeResult(context.Background(), id, result); err != nil {
						return err
					}
					return crash
				})
				return err
			})
			if !errors.Is(err, crash) {
				t.Fatal(err)
			}
			before, err := os.ReadFile(h.path)
			if err != nil {
				t.Fatal("observer failure removed intent")
			}
			h.mu.Lock()
			h.failMethod = "getTransaction"
			h.mu.Unlock()
			observer := func(txResult) error {
				t.Error("outage called observer")
				return nil
			}
			err = runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.resume(context.Background(), h.path, observer)
				return err
			})
			if err == nil {
				t.Fatal("metadata outage did not fail")
			}
			after, _ := os.ReadFile(h.path)
			if !bytes.Equal(before, after) {
				t.Fatal("metadata outage rewrote pending intent")
			}
			h.mu.Lock()
			h.failMethod = ""
			h.mu.Unlock()
			err = runDurablePayer(t, svc, func(p *payerSession) error {
				result, err := p.resume(context.Background(), h.path, func(result txResult) error {
					if result.fee != 5000 || result.compute != 123 || result.payerLamports == nil || *result.payerLamports != 5000 || string(result.failure) != failure {
						return fmt.Errorf("wrong result %+v", result)
					}
					return p.chargeResult(context.Background(), id, result)
				})
				if err == nil && result == nil {
					return fmt.Errorf("missing resumed result")
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := svc.store.readLedger()
			if err != nil || ledger.spent(id) != 5000 || len(ledger.entries[id].Signatures) != 1 {
				t.Fatalf("double charge %v %v", ledger, err)
			}
			if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("observed terminal intent remains")
			}
			h.mu.Lock()
			if len(h.broadcasts) != 1 || h.calls["simulateTransaction"] != 1 {
				t.Errorf("broadcasts %d simulations %d", len(h.broadcasts), h.calls["simulateTransaction"])
			}
			h.mu.Unlock()
		})
	}
}

func TestDurablePaymentLegacyExactReplayLostResponseAndResend(t *testing.T) {
	svc, h, _ := durablePaymentService(t)
	data := durableFixture(t, "ts-intent.json")
	writeFixture(t, h.path, data)
	h.lostResponse = true
	err := runDurablePayer(t, svc, func(p *payerSession) error {
		_, err := p.resume(context.Background(), h.path, func(txResult) error {
			t.Error("ambiguous send observed")
			return nil
		})
		return err
	})
	if err == nil {
		t.Fatal("lost response accepted")
	}
	after, _ := os.ReadFile(h.path)
	if !bytes.Equal(data, after) {
		t.Fatal("lost response changed legacy intent")
	}
	h.mu.Lock()
	h.lostResponse = false
	h.landed = false
	h.statusDelay = 18
	h.mu.Unlock()
	if err := runDurablePayer(t, svc, func(p *payerSession) error {
		_, err := p.resume(context.Background(), h.path, func(txResult) error { return nil })
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var original intent
	if err := json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.broadcasts) < 3 || h.calls["getLatestBlockhash"] != 0 || h.calls["simulateTransaction"] != 0 {
		t.Fatalf("resume recompiled or failed to resend: %+v", h.calls)
	}
	for _, raw := range h.broadcasts {
		if raw != original.Raw {
			t.Fatal("legacy transaction bytes changed on replay")
		}
	}
	if h.sendTimes[2].Sub(h.sendTimes[1]) < 2*time.Second {
		t.Fatal("resend earlier than 2 seconds")
	}
}

func TestDurablePaymentFinalityByExactStage(t *testing.T) {
	stages := []string{"lookup-create", "lookup-extend", "lookup-deactivate", "lookup-close", "assembly", "cleanup", "proof-create", "proof-grow", "proof-append", "proof-close", "resolve", "signature-batch-create", "signature", "chunk", "consensus-prune"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			svc, h, _ := durablePaymentService(t)
			var doc intent
			if err := json.Unmarshal(durableFixture(t, "ts-intent.json"), &doc); err != nil {
				t.Fatal(err)
			}
			doc.Stage = stage
			if err := svc.store.saveIntent(h.path, &doc); err != nil {
				t.Fatal(err)
			}
			h.status = "confirmed"
			h.landed = true
			observed := false
			err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.resume(context.Background(), h.path, func(txResult) error {
					observed = true
					return nil
				})
				return err
			})
			needsFinalized := strings.HasPrefix(stage, "lookup-") || strings.HasPrefix(stage, "proof-") || stage == "assembly" || stage == "cleanup" || stage == "resolve"
			if needsFinalized {
				if err == nil || observed || !strings.Contains(err.Error(), "confirmation unresolved") {
					t.Fatalf("confirmed settled %s: %v", stage, err)
				}
			} else if err != nil || !observed {
				t.Fatalf("confirmed did not settle %s: %v", stage, err)
			}
			h.mu.Lock()
			if len(h.broadcasts) != 0 {
				t.Error("confirmed transaction was resent")
			}
			h.mu.Unlock()
		})
	}
}

func TestDurablePaymentUnknownExpiredAndMissingMetadataRetainIntent(t *testing.T) {
	for _, mode := range []string{"expired", "metadata-null", "metadata-missing", "balance-missing", "status-outage"} {
		t.Run(mode, func(t *testing.T) {
			svc, h, _ := durablePaymentService(t)
			data := durableFixture(t, "ts-intent.json")
			writeFixture(t, h.path, data)
			switch mode {
			case "expired":
				h.blockHeight = 1001
			case "metadata-null":
				h.landed = true
				h.nullMetadata = 21
			case "metadata-missing":
				h.landed = true
				h.missingMetadata = true
			case "balance-missing":
				h.landed = true
				h.missingBalances = true
			case "status-outage":
				h.failMethod = "getSignatureStatuses"
			}
			for i := 0; i < 2; i++ {
				if mode == "metadata-null" {
					h.mu.Lock()
					h.nullMetadata += h.calls["getTransaction"]
					h.mu.Unlock()
				}
				err := runDurablePayer(t, svc, func(p *payerSession) error {
					_, err := p.resume(context.Background(), h.path, func(txResult) error {
						t.Error("unresolved observed")
						return nil
					})
					return err
				})
				if err == nil {
					t.Fatal("unresolved intent consumed")
				}
				after, _ := os.ReadFile(h.path)
				if !bytes.Equal(data, after) {
					t.Fatal("unresolved intent modified")
				}
			}
			h.mu.Lock()
			if len(h.broadcasts) != 0 {
				t.Error("expired or already landed intent rebroadcast")
			}
			h.mu.Unlock()
		})
	}
}

func TestDurablePaymentPreflightAndLiveGenesisNoSend(t *testing.T) {
	for _, mode := range []string{"simulation", "wrong-genesis-send", "wrong-genesis-settle", "durable-write", "pending"} {
		t.Run(mode, func(t *testing.T) {
			svc, h, request := durablePaymentService(t)
			switch mode {
			case "simulation":
				h.simulationFails = true
			case "wrong-genesis-send":
				h.genesis = h.identity.ProgramID
			case "wrong-genesis-settle":
				h.genesisFlipAt = 2
			case "durable-write":
				svc.store.writeJSON = func(string, []byte) error { return errors.New("disk unavailable") }
			case "pending":
				writeFixture(t, h.path, durableFixture(t, "ts-intent.json"))
			}
			err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.send(context.Background(), request, h.path, func(txResult) error {
					t.Error("failed send observed")
					return nil
				})
				return err
			})
			if err == nil {
				t.Fatal("invalid send accepted")
			}
			h.mu.Lock()
			if len(h.broadcasts) != 0 {
				t.Error("invalid send broadcast")
			}
			h.mu.Unlock()
			_, statErr := os.Stat(h.path)
			shouldRemain := mode == "wrong-genesis-settle" || mode == "pending"
			if shouldRemain != (statErr == nil) {
				t.Fatalf("wrong intent retention: %v", statErr)
			}
		})
	}
}

func TestDurablePaymentReclaimedRentAndPrune(t *testing.T) {
	svc, h, request := durablePaymentService(t)
	h.delta = -25000
	h.nullMetadata = 2
	request.stage = "proof-close"
	request.preflight = false
	if err := runDurablePayer(t, svc, func(p *payerSession) error {
		result, err := p.send(context.Background(), request, h.path, func(txResult) error { return nil })
		if err == nil && (result.payerLamports == nil || *result.payerLamports != -25000) {
			return fmt.Errorf("returned rent lost sign")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	h.path = ""
	h.status = "confirmed"
	h.landed = false
	h.mu.Unlock()
	if err := runDurablePayer(t, svc, func(p *payerSession) error {
		result, err := p.sendPrune(context.Background(), request.instructions)
		if err == nil && result.stage != "consensus-prune" {
			return fmt.Errorf("wrong prune stage")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.calls["simulateTransaction"] != 0 || len(h.broadcasts) != 2 || h.calls["getTransaction"] != 4 {
		t.Fatalf("prune/metadata behavior %+v", h.calls)
	}
}

func TestDurablePaymentIntentCorruptionAndWireLimits(t *testing.T) {
	mutations := map[string]func(*intent){
		"signature": func(v *intent) { v.Signature = "wrong" },
		"payer":     func(v *intent) { v.Payer = v.Genesis },
		"genesis":   func(v *intent) { v.Genesis = v.Payer },
		"base64":    func(v *intent) { v.Raw += "\n" },
		"raw":       func(v *intent) { v.Raw = "AA==" },
		"bytes":     func(v *intent) { v.Raw = base64.StdEncoding.EncodeToString(make([]byte, 1233)) },
		"accounts":  func(v *intent) { v.Accounts = 65 },
		"height":    func(v *intent) { v.LastValidBlockHeight = maxSafeInteger + 1 },
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			svc, h, _ := durablePaymentService(t)
			var doc intent
			if err := json.Unmarshal(durableFixture(t, "ts-intent.json"), &doc); err != nil {
				t.Fatal(err)
			}
			mutation(&doc)
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, h.path, data)
			if err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.resume(context.Background(), h.path, func(txResult) error {
					t.Error("corrupt observed")
					return nil
				})
				return err
			}); err == nil {
				t.Fatal("corrupt intent accepted")
			}
			h.mu.Lock()
			if len(h.calls) != 0 {
				t.Error("corrupt intent reached RPC")
			}
			h.mu.Unlock()
		})
	}
	for _, mode := range []string{"bytes", "accounts"} {
		t.Run("new-wire-"+mode, func(t *testing.T) {
			svc, h, request := durablePaymentService(t)
			program, _ := solana.PublicKeyFromBase58(h.identity.ProgramID)
			metas := solana.AccountMetaSlice{solana.Meta(svc.payer.PublicKey()).SIGNER().WRITE()}
			data := make([]byte, 1232)
			if mode == "accounts" {
				data = []byte{1}
				for i := 0; i < 64; i++ {
					hash := sha256.Sum256([]byte(fmt.Sprintf("test-account-%d", i)))
					metas = append(metas, solana.Meta(solana.PublicKey(hash)))
				}
			}
			request.instructions = []solana.Instruction{solana.NewInstruction(program, metas, data)}
			if err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.send(context.Background(), request, h.path, func(txResult) error {
					t.Error("oversize observed")
					return nil
				})
				return err
			}); err == nil {
				t.Fatal("oversize transaction accepted")
			}
			h.mu.Lock()
			if len(h.broadcasts) != 0 || h.calls["simulateTransaction"] != 0 {
				t.Error("oversize transaction reached simulation/send")
			}
			h.mu.Unlock()
			if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("oversize transaction journaled")
			}
		})
	}
}

func TestDurableSimulationPreservesRejectionReasonWithoutPayment(t *testing.T) {
	for _, failure := range []string{`"AccountNotFound"`, `{"InstructionError":[0,"InsufficientFunds"]}`} {
		t.Run(failure, func(t *testing.T) {
			svc, h, request := durablePaymentService(t)
			h.simulationError = json.RawMessage(failure)
			request.stage = "proof-create"
			request.preflight = true
			err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.send(context.Background(), request, h.path, func(txResult) error {
					t.Error("rejected simulation invoked payment observer")
					return nil
				})
				return err
			})
			if err == nil || err.Error() != "proof-create: simulation failed: "+failure {
				t.Fatalf("simulation reason lost: %v", err)
			}
			if len(h.broadcasts) != 0 || h.calls["sendTransaction"] != 0 {
				t.Fatal("rejected simulation sent a paid transaction")
			}
			if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("rejected simulation saved a signed intent")
			}
		})
	}
}

func TestResolutionBootstrapConfirmedSendPreflight(t *testing.T) {
	h := newResolutionHarness(t)
	instruction := system.NewTransferInstruction(1, h.s.payer.PublicKey(), solana.PublicKey{4}).Build()
	err := runDurablePayer(t, h.s, func(p *payerSession) error {
		result, err := p.sendBootstrap(context.Background(), []solana.Instruction{instruction})
		if err == nil && result.stage != "bootstrap" {
			t.Errorf("wrong bootstrap stage %q", result.stage)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if h.calls["simulateTransaction"] != 0 || h.calls["sendTransaction"] != 1 || h.calls["getGenesisHash"] != 2 {
		t.Fatalf("bootstrap RPC policy changed: %v", h.calls)
	}
	if len(h.stages) != 1 || h.stages[0] != "bootstrap" {
		t.Fatalf("unexpected bootstrap sends: %v", h.stages)
	}
	ids, err := h.s.store.proofJobIDs()
	if err != nil || len(ids) != 0 {
		t.Fatalf("bootstrap created proof jobs: %v %v", ids, err)
	}
}
