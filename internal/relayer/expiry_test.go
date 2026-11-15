package relayer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type expiryFixture struct {
	mu       sync.Mutex
	t        *testing.T
	service  *service
	doc      intent
	path     string
	id       sink.ID
	now      time.Time
	calls    map[string]int
	times    []time.Time
	anchor   uint64
	observed int
	mutate   func(string, []json.RawMessage, any) any
	reply    func(http.ResponseWriter, json.RawMessage, string) bool
	cancel   context.CancelFunc
}

func expiryTestHash(height uint64) solana.Hash {
	return solana.Hash(sha256.Sum256([]byte(fmt.Sprintf("expiry-r64-block-%d", height))))
}

func expiryTestSlot(height uint64) uint64 { return 1000 + 2*height }

func (f *expiryFixture) block(height uint64) map[string]any {
	seed := sha256.Sum256([]byte(fmt.Sprintf("expiry-r64-other-signer-%d", height)))
	key := ed25519.NewKeyFromSeed(seed[:])
	var sig solana.Signature
	copy(sig[:], ed25519.Sign(key, []byte("offline block member")))
	return map[string]any{
		"blockHeight":       height,
		"parentSlot":        expiryTestSlot(height - 1),
		"blockhash":         expiryTestHash(height).String(),
		"previousBlockhash": expiryTestHash(height - 1).String(),
		"signatures":        []string{sig.String()},
	}
}

func newExpiryFixture(t *testing.T, stage string) *expiryFixture {
	t.Helper()
	seed := sha256.Sum256([]byte("expiry-r64-fresh-offline-payer/" + t.Name()))
	payer := solana.PrivateKey(ed25519.NewKeyFromSeed(seed[:]))
	program := solana.PublicKey(sha256.Sum256([]byte("expiry-r64-program")))
	genesis := expiryTestHash(99000).String()
	identity := durableIdentity(t)
	store, err := newStateStore(durableTestDirectory(t), identity.ChainID, genesis, program.String(), payer.PublicKey().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := bridge.Derive(program)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := native.ParseID(identity.ID)
	f := &expiryFixture{
		t:      t,
		id:     id,
		now:    time.Unix(1800000000, 0),
		calls:  make(map[string]int),
		anchor: 1010,
	}
	s := &service{
		cfg: config{
			program:         program,
			chainID:         identity.ChainID,
			retryMaxSeconds: 60,
		},
		store:      store,
		payer:      payer,
		genesis:    genesis,
		deployment: deployment,
		deps:       defaultDependencies(Streams{}),
	}
	f.service = s
	s.deps.now = func() time.Time {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.now
	}
	s.deps.sleep = func(ctx context.Context, duration time.Duration) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.now = f.now.Add(duration)
		return ctx.Err()
	}
	var instruction solana.Instruction
	if strings.HasPrefix(stage, "proof-") || stage == "resolve" {
		var proof proofJob
		if err := json.Unmarshal(durableFixture(t, "ts-proof-transfer.json"), &proof); err != nil {
			t.Fatal(err)
		}
		proof.Genesis, proof.ProgramID, proof.Payer = genesis, program.String(), payer.PublicKey().String()
		if err := store.saveProof(&proof); err != nil {
			t.Fatal(err)
		}
		key, _ := hex.DecodeString(proof.Key)
		address, _ := deployment.ProofBuffer(payer.PublicKey(), key)
		instruction, err = deployment.AppendProof(payer.PublicKey(), address, 0, []byte{1, 2, 3})
		f.path, _ = store.paymentPath(id)
	} else {
		client := solana.PublicKey(sha256.Sum256([]byte("expiry-r64-light-client")))
		followed := bridge.Followed{Program: client}
		sessionHash := sha256.Sum256([]byte("expiry-r64-header-session/" + t.Name()))
		session := hex.EncodeToString(sessionHash[:])
		batch, _ := followed.BatchAt(payer.PublicKey(), sessionHash)
		instruction, err = followed.PreVerifySignatureBatch(payer.PublicKey(), batch, header.Receipt{})
		lookup := solana.PublicKey(sha256.Sum256([]byte("expiry-r64-lookup"))).String()
		slot := int64(200)
		doc := &sessionDoc{
			Version:    1,
			Genesis:    genesis,
			Payer:      payer.PublicKey().String(),
			Client:     client.String(),
			Target:     30,
			Trusted:    20,
			Session:    session,
			Lookup:     &lookup,
			LookupSlot: &slot,
		}
		if err := store.saveSession(session, doc); err != nil {
			t.Fatal(err)
		}
		f.path, _ = store.sessionPendingPath(session)
		if stage == "lookup-close" {
			f.path, _ = store.sessionClosePendingPath(session)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	tx, accounts, err := bridge.Compile(payer.PublicKey(), expiryTestHash(850), []solana.Instruction{instruction}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.doc = intent{
		Stage:                stage,
		Genesis:              genesis,
		Payer:                payer.PublicKey().String(),
		LastValidBlockHeight: 1000,
		Accounts:             accounts,
	}
	f.saveTransaction(tx)
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	s.solana = rpc.New(server.URL)
	s.cfg.solanaRPC = server.URL
	return f
}

func (f *expiryFixture) saveTransaction(tx *solana.Transaction) {
	f.t.Helper()
	if _, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key == f.service.payer.PublicKey() {
			return &f.service.payer
		}
		return nil
	}); err != nil {
		f.t.Fatal(err)
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		f.t.Fatal(err)
	}
	f.doc.Raw = base64.StdEncoding.EncodeToString(raw)
	f.doc.Signature = tx.Signatures[0].String()
	if err := f.service.store.saveIntent(f.path, &f.doc); err != nil {
		f.t.Fatal(err)
	}
}

func (f *expiryFixture) serve(w http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var req struct {
		ID     json.RawMessage   `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		f.t.Error(err)
		return
	}
	f.calls[req.Method]++
	f.times = append(f.times, f.now)
	if f.reply != nil && f.reply(w, req.ID, req.Method) {
		return
	}
	var result any
	switch req.Method {
	case "getGenesisHash":
		result = f.doc.Genesis
	case "getSignatureStatuses":
		if !bytes.Contains(req.Params[1], []byte(`"searchTransactionHistory":true`)) {
			f.t.Error("status history option missing")
		}
		result = map[string]any{
			"context": map[string]any{"slot": expiryTestSlot(f.anchor)},
			"value":   []any{nil},
		}
	case "getTransaction":
		if !bytes.Contains(req.Params[1], []byte(`"finalized"`)) || !bytes.Contains(req.Params[1], []byte(`"base64"`)) {
			f.t.Error("receipt is not strict finalized base64")
		}
		result = nil
	case "getBalance":
		result = map[string]any{
			"context": map[string]any{"slot": expiryTestSlot(f.anchor)},
			"value":   uint64(1000000000),
		}
	case "getAccountInfo":
		result = map[string]any{
			"context": map[string]any{"slot": expiryTestSlot(f.anchor)},
			"value":   nil,
		}
	case "getLatestBlockhash":
		result = map[string]any{
			"context": map[string]any{"slot": expiryTestSlot(f.anchor)},
			"value": map[string]any{
				"blockhash":            expiryTestHash(850).String(),
				"lastValidBlockHeight": 1000,
			},
		}
	case "getBlockHeight":
		result = f.anchor
	case "getSlot":
		if !bytes.Contains(req.Params[0], []byte(`"finalized"`)) {
			f.t.Error("anchor is not finalized")
		}
		result = expiryTestSlot(f.anchor)
	case "getBlock":
		var slot uint64
		json.Unmarshal(req.Params[0], &slot)
		if !bytes.Contains(req.Params[1], []byte(`"signatures"`)) || !bytes.Contains(req.Params[1], []byte(`"finalized"`)) {
			f.t.Error("block list must use full signatures finalized")
		}
		if slot <= 1000 || slot%2 != 0 {
			f.t.Error("locator requested skipped or invalid slot")
		} else {
			result = f.block((slot - 1000) / 2)
		}
	case "getBlocks":
		var start, end uint64
		json.Unmarshal(req.Params[0], &start)
		json.Unmarshal(req.Params[1], &end)
		if end < start || end-start > 4096 || !bytes.Contains(req.Params[2], []byte(`"finalized"`)) {
			f.t.Error("unbounded or nonfinalized locator")
		}
		slots := []uint64{}
		for slot := max(uint64(1002), start); slot <= end; slot++ {
			if slot%2 == 0 {
				slots = append(slots, slot)
			}
		}
		result = slots
	case "isBlockhashValid":
		if !bytes.Contains(req.Params[1], []byte(`"minContextSlot":3004`)) || !bytes.Contains(req.Params[1], []byte(`"finalized"`)) {
			f.t.Error("hash validity must be anchored to window end")
		}
		result = map[string]any{
			"context": map[string]any{"slot": expiryTestSlot(f.anchor)},
			"value":   false,
		}
	default:
		f.t.Errorf("unexpected expiry RPC method %s", req.Method)
	}
	if f.mutate != nil {
		result = f.mutate(req.Method, req.Params, result)
	}
	json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  result,
	})
}

func (f *expiryFixture) resume(ctx context.Context) error {
	return runDurablePayer(f.t, f.service, func(p *payerSession) error {
		result, err := p.resume(ctx, f.path, func(txResult) error {
			f.observed++
			return nil
		})
		if result != nil {
			f.t.Error("certification invented a transaction result")
		}
		return err
	})
}

func (f *expiryFixture) retained(ctx context.Context) error {
	f.t.Helper()
	before, _ := os.ReadFile(f.path)
	err := f.resume(ctx)
	after, readErr := os.ReadFile(f.path)
	if err == nil || isIntentNotIncluded(err) || readErr != nil || !bytes.Equal(before, after) || f.observed != 0 || f.calls["sendTransaction"] != 0 {
		f.t.Fatalf("unsafe expiry outcome: err=%v retained=%v observed=%d calls=%v", err, bytes.Equal(before, after), f.observed, f.calls)
	}
	return err
}

func TestExpiryOriginalUnknownHistoryRecovery(t *testing.T) {
	for _, stage := range []string{"proof-append", "signature"} {
		t.Run(stage, func(t *testing.T) {
			f := newExpiryFixture(t, stage)
			before, _ := os.ReadFile(f.path)
			saved, err := f.service.store.snapshotPending(f.path)
			if err != nil {
				t.Fatal(err)
			}
			err = f.resume(context.Background())
			if !isIntentNotIncluded(err) {
				t.Fatalf("original expired unknown history still blocks automatic recovery: %v", err)
			}
			if _, err := os.Stat(f.path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("certified pending was not retired")
			}
			cert, err := f.service.store.loadExpiryCertificate(saved)
			if err != nil || cert == nil {
				t.Fatalf("immutable certificate missing: %v", err)
			}
			if len(cert.Blocks) != 165 || cert.Blocks[152].Blockhash != expiryTestHash(850).String() || !bytes.Equal(saved.Original, before) || f.observed != 0 || f.calls["sendTransaction"] != 0 || f.calls["simulateTransaction"] != 0 {
				t.Fatal("expiry did not preserve full evidence or bypassed accounting")
			}
			if err := f.resume(context.Background()); err != nil {
				t.Fatalf("next pass cannot continue: %v", err)
			}
		})
	}
}

func TestExpiryRetainsContradictoryWindows(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*expiryFixture, uint64, map[string]any)
	}{
		{"origin-included", func(f *expiryFixture, height uint64, b map[string]any) {
			if height == 850 {
				b["signatures"] = []string{f.doc.Signature}
			}
		}},
		{"interior-included", func(f *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				b["signatures"] = []string{f.doc.Signature}
			}
		}},
		{"expiry-included", func(f *expiryFixture, height uint64, b map[string]any) {
			if height == 1000 {
				b["signatures"] = []string{f.doc.Signature}
			}
		}},
		{"first-boundary-included", func(f *expiryFixture, height uint64, b map[string]any) {
			if height == 838 {
				b["signatures"] = []string{f.doc.Signature}
			}
		}},
		{"last-boundary-included", func(f *expiryFixture, height uint64, b map[string]any) {
			if height == 1002 {
				b["signatures"] = []string{f.doc.Signature}
			}
		}},
		{"parent-gap", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				b["parentSlot"] = expiryTestSlot(898)
			}
		}},
		{"height-gap", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				b["blockHeight"] = 899
			}
		}},
		{"hash-gap", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				b["previousBlockhash"] = expiryTestHash(700).String()
			}
		}},
		{"duplicate-origin", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				b["blockhash"] = expiryTestHash(850).String()
			}
		}},
		{"orphan-origin", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 850 {
				b["blockhash"] = expiryTestHash(700).String()
			}
		}},
		{"empty-signatures", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				b["signatures"] = []string{}
			}
		}},
		{"missing-signatures", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				delete(b, "signatures")
			}
		}},
		{"noncanonical-signature", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				b["signatures"] = []string{"bad"}
			}
		}},
		{"null-height", func(_ *expiryFixture, height uint64, b map[string]any) {
			if height == 900 {
				b["blockHeight"] = nil
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			f.mutate = func(method string, _ []json.RawMessage, result any) any {
				if method == "getBlock" {
					block := result.(map[string]any)
					tc.mutate(f, block["blockHeight"].(uint64), block)
				}
				return result
			}
			f.retained(context.Background())
		})
	}
}

func TestExpiryOrdinaryTransactionBoundary(t *testing.T) {
	for _, stage := range []string{"proof-create", "proof-grow", "proof-append", "proof-close", "resolve", "signature-batch-create", "signature", "chunk", "assembly", "cleanup", "lookup-create", "lookup-extend", "lookup-deactivate", "lookup-close"} {
		t.Run(stage, func(t *testing.T) {
			f := newExpiryFixture(t, stage)
			if _, err := f.service.store.snapshotPending(f.path); err != nil {
				t.Fatal(err)
			}
			if err := f.resume(context.Background()); !isIntentNotIncluded(err) {
				t.Fatal(err)
			}
		})
	}
	for _, kind := range []string{"system", "nonce", "bad-crypto", "trailing-wire", "bad-index", "bad-program", "account-count", "bad-alt-index", "unsupported-stage", "age-underflow"} {
		t.Run(kind, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			raw, _ := base64.StdEncoding.DecodeString(f.doc.Raw)
			tx, _ := solana.TransactionFromBytes(raw)
			switch kind {
			case "system", "nonce":
				ix := system.NewTransferInstruction(1, f.service.payer.PublicKey(), f.service.cfg.program).Build()
				if kind == "nonce" {
					ix = system.NewAdvanceNonceAccountInstruction(f.service.cfg.program, solana.SysVarRecentBlockHashesPubkey, f.service.payer.PublicKey()).Build()
				}
				var err error
				tx, f.doc.Accounts, err = bridge.Compile(f.service.payer.PublicKey(), expiryTestHash(850), []solana.Instruction{ix}, nil)
				if err != nil {
					t.Fatal(err)
				}
				f.saveTransaction(tx)
			case "bad-crypto":
				tx.Signatures[0][0] ^= 1
				wire, _ := tx.MarshalBinary()
				f.doc.Raw = base64.StdEncoding.EncodeToString(wire)
				f.doc.Signature = tx.Signatures[0].String()
			case "trailing-wire":
				f.doc.Raw = base64.StdEncoding.EncodeToString(append(raw, 1))
			case "bad-index":
				tx.Message.Instructions[0].Accounts[0] = 63
				f.saveTransaction(tx)
			case "bad-program":
				tx.Message.Instructions[0].ProgramIDIndex = 63
				f.saveTransaction(tx)
			case "bad-alt-index":
				tx.Message.AddressTableLookups = []solana.MessageAddressTableLookup{
					{
						AccountKey:      f.service.cfg.program,
						WritableIndexes: []uint8{1, 1},
					},
				}
				f.doc.Accounts += 2
				f.saveTransaction(tx)
			case "account-count":
				f.doc.Accounts++
			case "unsupported-stage":
				f.doc.Stage = "bootstrap"
			case "age-underflow":
				f.doc.LastValidBlockHeight = 10
			}
			if _, err := validateOrdinaryIntent(&f.doc); err == nil {
				t.Fatal("unsafe transaction admitted")
			}
		})
	}
}

func TestExpiryRPCBoundaryRetainsIntent(t *testing.T) {
	cases := []string{"status-present", "status-missing-context", "stale-final-status", "receipt-present", "genesis-change", "anchor-regression", "hash-valid", "hash-null", "hash-missing", "hash-stale", "null-block", "age-151", "method-error", "wrong-id", "missing-result", "truncated", "oversize", "duplicate-json", "rate-limit", "cancel"}
	for _, kind := range cases {
		t.Run(kind, func(t *testing.T) {
			f := newExpiryFixture(t, "signature")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.mutate = func(method string, _ []json.RawMessage, result any) any {
				switch kind {
				case "status-present":
					if method == "getSignatureStatuses" && f.calls[method] >= 3 {
						result.(map[string]any)["value"] = []any{map[string]any{"slot": 3000}}
					}
				case "status-missing-context":
					if method == "getSignatureStatuses" {
						delete(result.(map[string]any), "context")
					}
				case "stale-final-status":
					if method == "getSignatureStatuses" && f.calls[method] >= 4 {
						result.(map[string]any)["context"] = map[string]any{"slot": 1}
					}
				case "receipt-present":
					if method == "getTransaction" && f.calls[method] >= 2 {
						return map[string]any{"slot": 3000}
					}
				case "genesis-change":
					if method == "getGenesisHash" && f.calls[method] >= 3 {
						return expiryTestHash(55).String()
					}
				case "anchor-regression":
					if method == "getSlot" && f.calls[method] >= 2 {
						return expiryTestSlot(1009)
					}
				case "hash-valid":
					if method == "isBlockhashValid" {
						result.(map[string]any)["value"] = true
					}
				case "hash-null":
					if method == "isBlockhashValid" {
						result.(map[string]any)["value"] = nil
					}
				case "hash-missing":
					if method == "isBlockhashValid" {
						delete(result.(map[string]any), "value")
					}
				case "hash-stale":
					if method == "isBlockhashValid" {
						result.(map[string]any)["context"] = map[string]any{"slot": 1}
					}
				case "null-block":
					if method == "getBlock" {
						return nil
					}
				case "cancel":
					if method == "getBlock" {
						cancel()
					}
				}
				return result
			}
			if kind == "age-151" {
				f.doc.LastValidBlockHeight++
				f.service.store.saveIntent(f.path, &f.doc)
			}
			f.reply = func(w http.ResponseWriter, id json.RawMessage, method string) bool {
				if method != "getBlock" {
					return false
				}
				switch kind {
				case "method-error":
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"unavailable"}}`, id)
				case "wrong-id":
					fmt.Fprint(w, `{"jsonrpc":"2.0","id":"wrong","result":null}`)
				case "missing-result":
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s}`, id)
				case "truncated":
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":`, id)
				case "oversize":
					fmt.Fprint(w, strings.Repeat(" ", expiryMaxResponse+1))
				case "duplicate-json":
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":null,"result":null}`, id)
				case "rate-limit":
					w.Header().Set("Retry-After", "900")
					w.WriteHeader(429)
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":429,"message":"limited"}}`, id)
				default:
					return false
				}
				return true
			}
			f.retained(ctx)
		})
	}
}

func TestExpiryMarkerSkipsRetryAccounting(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	marker := &intentNotIncludedError{digest: "test"}
	if err := f.service.relayFailed(context.Background(), f.id, marker); err != marker {
		t.Fatal("relay marker swallowed")
	}
	before := f.service.headerRetry
	ready, err := f.service.headerStep(func() error { return marker }, 12)
	if ready || err != marker || f.service.headerRetry != before {
		t.Fatal("header marker treated as retry or success")
	}
	ledger, err := f.service.store.readLedger()
	if err != nil || len(ledger.entries) != 0 {
		t.Fatal("marker changed accounting")
	}
}

func TestExpiryCapsuleRestartAndFaults(t *testing.T) {
	for _, fault := range []string{"write", "short-write", "file-sync", "rename", "ancestor-sync", "capsule-directory-sync", "unlink", "pending-directory-sync", "disk-change", "symlink"} {
		t.Run(fault, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			saved, err := f.service.store.snapshotPending(f.path)
			if err != nil {
				t.Fatal(err)
			}
			var cert *nonInclusionCertificate
			err = runDurablePayer(t, f.service, func(p *payerSession) error {
				cert, err = p.certifyExpired(context.Background(), saved)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			ops := expiryFiles()
			injected := errors.New("test expiry publication fault")
			published := false
			ops.write = func(file *os.File, data []byte) (int, error) {
				if fault == "write" {
					return 0, injected
				}
				if fault == "short-write" {
					return file.Write(data[:len(data)/2])
				}
				return file.Write(data)
			}
			ops.rename = func(from, to string) error {
				if fault == "rename" {
					return injected
				}
				if err := os.Rename(from, to); err != nil {
					return err
				}
				published = true
				if fault == "disk-change" {
					return os.WriteFile(f.path, append(saved.Original, '\n'), 0o600)
				}
				if fault == "symlink" {
					os.Remove(f.path)
					return os.Symlink(to, f.path)
				}
				return nil
			}
			ops.sync = func(file *os.File) error {
				info, _ := file.Stat()
				if fault == "file-sync" && info.Mode().IsRegular() || fault == "ancestor-sync" && file.Name() == f.service.store.expiryRoot() || fault == "capsule-directory-sync" && published || fault == "pending-directory-sync" && file.Name() == filepath.Dir(f.path) {
					return injected
				}
				return file.Sync()
			}
			ops.unlink = func(path string) error {
				if fault == "unlink" {
					return injected
				}
				return os.Remove(path)
			}
			err = runDurablePayer(t, f.service, func(p *payerSession) error {
				return p.retireExpiredWith(context.Background(), saved, cert, ops)
			})
			if err == nil {
				t.Fatal("fault was ignored")
			}
			if f.observed != 0 {
				t.Fatal("publication observed accounting")
			}
			if fault == "pending-directory-sync" {
				if _, err := os.Stat(f.path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("unlink did not precede sync fault")
				}
				return
			}
			if _, err := os.Lstat(f.path); err != nil {
				t.Fatal("fault lost pending")
			}
			if fault == "disk-change" || fault == "symlink" {
				return
			}
			blocks := f.calls["getBlock"]
			f.service.expiryCooldowns = nil
			err = f.resume(context.Background())
			if !isIntentNotIncluded(err) {
				t.Fatalf("restart failed: %v", err)
			}
			if published && f.calls["getBlock"]-blocks != 1 {
				t.Fatal("cached certificate unnecessarily rescanned window")
			}
		})
	}
}

func TestExpiryCooldownIsBoundedAndUsesExactBytes(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	f.mutate = func(method string, _ []json.RawMessage, value any) any {
		if method == "getBlock" {
			return nil
		}
		return value
	}
	f.retained(context.Background())
	blocks := f.calls["getBlock"]
	for i := 0; i < 20; i++ {
		f.mu.Lock()
		f.now = f.now.Add(3 * time.Second)
		f.mu.Unlock()
		f.retained(context.Background())
		if i < 19 && f.calls["getBlock"] != blocks {
			t.Fatal("repeated tick rescanned before cooldown")
		}
	}
	if f.calls["getBlock"] != blocks+1 {
		t.Fatal("cooldown did not allow later retry")
	}
	data, _ := os.ReadFile(f.path)
	os.WriteFile(f.path, append(data, '\n'), 0o600)
	f.retained(context.Background())
	if f.calls["getBlock"] != blocks+2 {
		t.Fatal("changed exact bytes shared old cooldown")
	}
	for i := 0; i < 1000; i++ {
		f.service.expiryFailed(fmt.Sprint(i), 0)
	}
	if len(f.service.expiryCooldowns) != expiryCooldownSize {
		t.Fatal("unbounded cooldown cache")
	}
}

func TestExpiryCertificateCorruptionRetains(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	saved, _ := f.service.store.snapshotPending(f.path)
	if err := runDurablePayer(t, f.service, func(p *payerSession) error {
		cert, err := p.certifyExpired(context.Background(), saved)
		if err != nil {
			return err
		}
		return f.service.store.publishExpiry(saved, cert, expiryFiles())
	}); err != nil {
		t.Fatal(err)
	}
	path := f.service.store.expiryCapsulePath(saved)
	data, _ := os.ReadFile(path)
	data = bytes.Replace(data, []byte(`"maxProcessingAge":150`), []byte(`"maxProcessingAge":151`), 1)
	os.WriteFile(path, data, 0o600)
	blocks := f.calls["getBlock"]
	f.retained(context.Background())
	if f.calls["getBlock"] != blocks {
		t.Fatal("corrupt capsule was ignored and replaced")
	}
}

func TestExpiryObserverContradictions(t *testing.T) {
	for _, where := range []string{"ledger", "session", "journal", "submitted"} {
		t.Run(where, func(t *testing.T) {
			f := newExpiryFixture(t, "signature")
			s := f.service.store
			switch where {
			case "ledger":
				_, err := s.updateLedger(context.Background(), func(l *spendLedger) (bool, error) { return true, l.charge(f.id, 5000, f.doc.Signature, 0) })
				if err != nil {
					t.Fatal(err)
				}
			case "session":
				id := strings.TrimSuffix(filepath.Base(f.path), ".pending.json")
				doc, _ := s.loadSession(id)
				doc.Assembly = transactionDocument(txResult{
					stage:     "assembly",
					signature: f.doc.Signature,
					failure:   json.RawMessage(`null`),
				})
				if err := s.saveSession(id, doc); err != nil {
					t.Fatal(err)
				}
			default:
				event := "transaction"
				if where == "submitted" {
					event = "submitted"
				}
				if err := s.appendHeaderEvent(map[string]any{"event": event, "signature": f.doc.Signature}); err != nil {
					t.Fatal(err)
				}
			}
			if where == "submitted" {
				if err := f.resume(context.Background()); !isIntentNotIncluded(err) {
					t.Fatal(err)
				}
			} else {
				f.retained(context.Background())
			}
		})
	}
}

func TestExpiryCertificatePreservesOtherDocuments(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	_, err := f.service.store.updateLedger(context.Background(), func(l *spendLedger) (bool, error) {
		if err := l.charge(f.id, 15000, "previous", 100000); err != nil {
			return false, err
		}
		if err := l.attempt(f.id); err != nil {
			return false, err
		}
		_, _, err := l.failed(f.id, f.now.UnixMilli(), 60000)
		return true, err
	})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{f.service.store.ledgerPath, f.path + ".proof"}
	before := make(map[string][]byte)
	for _, path := range paths {
		before[path], _ = os.ReadFile(path)
	}
	if err := f.resume(context.Background()); !isIntentNotIncluded(err) {
		t.Fatal(err)
	}
	for path, original := range before {
		after, _ := os.ReadFile(path)
		if !bytes.Equal(original, after) {
			t.Fatal("certification mutated job or accounting")
		}
	}
	ids, err := f.service.store.proofJobIDs()
	if err != nil || !reflect.DeepEqual(ids, []sink.ID{f.id}) {
		t.Fatal("capsule polluted legacy enumeration")
	}
}
