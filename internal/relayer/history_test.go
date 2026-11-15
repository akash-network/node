package relayer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type historyRPC struct {
	*durableRPC
	doc     intent
	receipt map[string]any
	reply   func(http.ResponseWriter, *http.Request, json.RawMessage)
}

func newHistoryRecovery(t *testing.T, stage string, failure json.RawMessage, delta int64) (*service, *historyRPC) {
	t.Helper()
	svc, base, _ := durablePaymentService(t)
	var doc intent
	if err := json.Unmarshal(durableFixture(t, "ts-intent.json"), &doc); err != nil {
		t.Fatal(err)
	}
	doc.Stage = stage
	if err := svc.store.saveIntent(base.path, &doc); err != nil {
		t.Fatal(err)
	}
	raw, _, err := doc.transaction()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := solana.TransactionFromBytes(raw)
	if err != nil || tx.VerifySignatures() != nil {
		t.Fatal("fixture must contain a genuine synthetic signed transaction")
	}
	pre := make([]int64, doc.Accounts)
	post := make([]int64, doc.Accounts)
	pre[0], post[0] = 10000000, 10000000-delta
	base.blockHeight = uint64(doc.LastValidBlockHeight + 1)
	h := &historyRPC{
		durableRPC: base,
		doc:        doc,
		receipt: map[string]any{
			"slot":        20,
			"version":     0,
			"transaction": []string{doc.Raw, "base64"},
			"meta": map[string]any{
				"err":                  failure,
				"fee":                  5000,
				"computeUnitsConsumed": 123,
				"preBalances":          pre,
				"postBalances":         post,
				"logMessages":          []string{"test execution"},
				"loadedAddresses": map[string]any{
					"writable": []string{},
					"readonly": []string{},
				},
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(h.serveHistory))
	t.Cleanup(server.Close)
	svc.solana = rpc.New(server.URL)
	return svc, h
}

func (h *historyRPC) serveHistory(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.t.Error(err)
		return
	}
	var request struct {
		ID     json.RawMessage   `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		h.t.Error(err)
		return
	}
	if request.Method != "getTransaction" {
		r.Body = io.NopCloser(bytes.NewReader(body))
		h.durableRPC.serve(w, r)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls[request.Method]++
	var signature string
	var options map[string]any
	if len(request.Params) != 2 || json.Unmarshal(request.Params[0], &signature) != nil || json.Unmarshal(request.Params[1], &options) != nil {
		h.t.Error("invalid history request parameters")
		return
	}
	if signature != h.doc.Signature || options["commitment"] != "finalized" || options["encoding"] != "base64" || options["maxSupportedTransactionVersion"] != float64(0) {
		h.t.Error("history lookup must request the original signature, finalized base64, version zero")
	}
	if h.reply != nil {
		h.reply(w, r, request.ID)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      request.ID,
		"result":  h.receipt,
	}); err != nil {
		h.t.Error(err)
	}
}

func TestHistoryRecoveryExactOriginalReceipt(t *testing.T) {
	for _, stage := range []string{"signature", "proof-append", "resolve"} {
		for _, failure := range []string{`null`, `{"InstructionError":[0,"InvalidArgument"]}`} {
			t.Run(stage+"/"+failure, func(t *testing.T) {
				svc, h := newHistoryRecovery(t, stage, json.RawMessage(failure), 5000)
				id, _ := native.ParseID(h.identity.ID)
				original, err := os.ReadFile(h.path)
				if err != nil {
					t.Fatal(err)
				}
				observed := 0
				err = runDurablePayer(t, svc, func(p *payerSession) error {
					result, err := p.resume(context.Background(), h.path, func(result txResult) error {
						observed++
						pending, readErr := os.ReadFile(h.path)
						if readErr != nil || !bytes.Equal(pending, original) {
							t.Error("observer must see the unchanged original intent")
						}
						if result.signature != h.doc.Signature || result.stage != stage || result.fee != 5000 || result.compute != 123 || result.payerLamports == nil || *result.payerLamports != 5000 || string(result.failure) != failure {
							t.Error("original execution accounting or failure was lost")
						}
						return p.chargeResult(context.Background(), id, result)
					})
					if err == nil && result == nil {
						t.Error("missing resumed receipt")
					}
					return err
				})
				if err != nil {
					t.Fatalf("complete original receipt was not recovered: %v; lookups=%d", err, h.calls["getTransaction"])
				}
				ledger, err := svc.store.readLedger()
				if err != nil || ledger.spent(id) != 5000 || len(ledger.entries[id].Signatures) != 1 || ledger.entries[id].Signatures[0] != h.doc.Signature {
					t.Fatal("exact receipt was not charged once")
				}
				if observed != 1 || h.calls["getTransaction"] != 1 || h.calls["sendTransaction"] != 0 || h.calls["getLatestBlockhash"] != 0 || h.calls["simulateTransaction"] != 0 {
					t.Fatalf("unexpected observer or RPC work: observed=%d calls=%v", observed, h.calls)
				}
				if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("observed intent was not removed")
				}
			})
		}
	}
}

func assertHistoryRetained(t *testing.T, svc *service, h *historyRPC, ctx context.Context) error {
	t.Helper()
	before, err := os.ReadFile(h.path)
	if err != nil {
		t.Fatal(err)
	}
	ledgerBefore, err := optionalJSON(svc.store.ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := native.ParseID(h.identity.ID)
	observed := 0
	err = runDurablePayer(t, svc, func(p *payerSession) error {
		result, err := p.resume(ctx, h.path, func(result txResult) error {
			observed++
			return p.chargeResult(ctx, id, result)
		})
		if result != nil {
			t.Error("unresolved receipt produced a result")
		}
		return err
	})
	if err == nil || observed != 0 {
		t.Fatalf("unresolved history accepted: observed=%d error=%v", observed, err)
	}
	after, readErr := os.ReadFile(h.path)
	ledgerAfter, ledgerErr := optionalJSON(svc.store.ledgerPath)
	if readErr != nil || ledgerErr != nil || !bytes.Equal(before, after) || !bytes.Equal(ledgerBefore, ledgerAfter) {
		t.Fatal("unresolved receipt changed signed intent or accounting")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.calls["getTransaction"] > 1 || h.calls["sendTransaction"] != 0 || h.calls["getLatestBlockhash"] != 0 || h.calls["simulateTransaction"] != 0 {
		t.Fatalf("unresolved history retried or paid: %v", h.calls)
	}
	return err
}

func TestHistoryRecoveryMalformedReceiptRetainsOriginal(t *testing.T) {
	mutations := map[string]func(*historyRPC){
		"missing-slot":        func(h *historyRPC) { delete(h.receipt, "slot") },
		"null-slot":           func(h *historyRPC) { h.receipt["slot"] = nil },
		"negative-slot":       func(h *historyRPC) { h.receipt["slot"] = -1 },
		"unsafe-slot":         func(h *historyRPC) { h.receipt["slot"] = maxSafeInteger + 1 },
		"string-slot":         func(h *historyRPC) { h.receipt["slot"] = "20" },
		"fraction-slot":       func(h *historyRPC) { h.receipt["slot"] = 20.5 },
		"missing-meta":        func(h *historyRPC) { delete(h.receipt, "meta") },
		"null-meta":           func(h *historyRPC) { h.receipt["meta"] = nil },
		"array-meta":          func(h *historyRPC) { h.receipt["meta"] = []any{} },
		"missing-version":     func(h *historyRPC) { delete(h.receipt, "version") },
		"null-version":        func(h *historyRPC) { h.receipt["version"] = nil },
		"unsupported-version": func(h *historyRPC) { h.receipt["version"] = 1 },
		"string-version":      func(h *historyRPC) { h.receipt["version"] = "0" },
		"legacy-conflict":     func(h *historyRPC) { h.receipt["version"] = "legacy" },
		"negative-version":    func(h *historyRPC) { h.receipt["version"] = -1 },
		"missing-transaction": func(h *historyRPC) { delete(h.receipt, "transaction") },
		"null-transaction":    func(h *historyRPC) { h.receipt["transaction"] = nil },
		"parsed-transaction": func(h *historyRPC) {
			h.receipt["transaction"] = map[string]any{"signatures": []string{h.doc.Signature}}
		},
		"truncated-tuple":     func(h *historyRPC) { h.receipt["transaction"] = []string{h.doc.Raw} },
		"extended-tuple":      func(h *historyRPC) { h.receipt["transaction"] = []string{h.doc.Raw, "base64", "extra"} },
		"wrong-encoding":      func(h *historyRPC) { h.receipt["transaction"] = []string{h.doc.Raw, "base58"} },
		"null-encoding":       func(h *historyRPC) { h.receipt["transaction"] = []any{h.doc.Raw, nil} },
		"wrong-raw":           func(h *historyRPC) { h.receipt["transaction"] = []string{"AA==", "base64"} },
		"null-raw":            func(h *historyRPC) { h.receipt["transaction"] = []any{nil, "base64"} },
		"noncanonical-base64": func(h *historyRPC) { h.receipt["transaction"] = []string{h.doc.Raw + "\n", "base64"} },
	}
	metaMutations := map[string]any{
		"err":             []any{nil},
		"fee":             -1,
		"preBalances":     []any{nil},
		"postBalances":    []any{nil},
		"logMessages":     []any{nil},
		"loadedAddresses": []any{},
	}
	for field, wrong := range metaMutations {
		mutations["missing-"+field] = func(h *historyRPC) { delete(h.receipt["meta"].(map[string]any), field) }
		mutations["malformed-"+field] = func(h *historyRPC) { h.receipt["meta"].(map[string]any)[field] = wrong }
		if field != "err" {
			mutations["null-"+field] = func(h *historyRPC) { h.receipt["meta"].(map[string]any)[field] = nil }
		}
	}
	for _, field := range []string{"fee", "computeUnitsConsumed"} {
		for name, value := range map[string]any{"negative": -1, "unsafe": maxSafeInteger + 1, "string": "5000", "bool": false, "fraction": 0.5, "object": map[string]any{}} {
			mutations[name+"-"+field] = func(h *historyRPC) { h.receipt["meta"].(map[string]any)[field] = value }
		}
	}
	for _, field := range []string{"preBalances", "postBalances"} {
		for name, value := range map[string]any{"negative": -1, "unsafe": maxSafeInteger + 1, "string": "0", "null": nil, "fraction": 0.5} {
			mutations[name+"-"+field+"-nonpayer"] = func(h *historyRPC) {
				balances := make([]any, h.doc.Accounts)
				for i := range balances {
					balances[i] = 10000000
				}
				balances[len(balances)-1] = value
				h.receipt["meta"].(map[string]any)[field] = balances
			}
		}
		mutations["truncated-"+field] = func(h *historyRPC) { h.receipt["meta"].(map[string]any)[field] = []int64{10000000} }
		mutations["extra-"+field] = func(h *historyRPC) { h.receipt["meta"].(map[string]any)[field] = make([]int64, h.doc.Accounts+1) }
	}
	for name, value := range map[string]any{
		"false":           false,
		"zero":            0,
		"empty-string":    "",
		"empty-object":    map[string]any{},
		"null-tag":        map[string]any{"InstructionError": nil},
		"bool-tag":        map[string]any{"InstructionError": false},
		"short-tag":       map[string]any{"InstructionError": []any{0}},
		"bad-instruction": map[string]any{"InstructionError": []any{900, "InvalidArgument"}},
		"bad-reason":      map[string]any{"InstructionError": []any{0, false}},
		"unsafe-custom": map[string]any{
			"InstructionError": []any{0, map[string]any{"Custom": uint64(1) << 32}},
		},
	} {
		mutations["error-"+name] = func(h *historyRPC) { h.receipt["meta"].(map[string]any)["err"] = value }
	}
	for _, field := range []string{"writable", "readonly"} {
		mutations["missing-loaded-"+field] = func(h *historyRPC) {
			delete(h.receipt["meta"].(map[string]any)["loadedAddresses"].(map[string]any), field)
		}
		mutations["null-loaded-"+field] = func(h *historyRPC) {
			h.receipt["meta"].(map[string]any)["loadedAddresses"].(map[string]any)[field] = nil
		}
		mutations["extra-loaded-"+field] = func(h *historyRPC) {
			h.receipt["meta"].(map[string]any)["loadedAddresses"].(map[string]any)[field] = []string{h.doc.Payer}
		}
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			svc, h := newHistoryRecovery(t, "proof-append", json.RawMessage(`null`), 5000)
			mutation(h)
			err := assertHistoryRetained(t, svc, h, context.Background())
			if !strings.Contains(err.Error(), "invalid finalized transaction receipt") || h.calls["getTransaction"] != 1 {
				t.Fatalf("wrong receipt rejection: %v calls=%v", err, h.calls)
			}
		})
	}
}

func TestHistoryRecoveryAbsenceTransportCancellationAndGenesis(t *testing.T) {
	for _, mode := range []string{"null", "missing-result", "malformed-json", "array-result", "http", "rpc", "cancel", "deadline", "genesis-entry", "genesis-after-receipt", "genesis-outage"} {
		t.Run(mode, func(t *testing.T) {
			svc, h := newHistoryRecovery(t, "signature", json.RawMessage(`null`), 5000)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "null":
				h.receipt = nil
			case "missing-result":
				h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s}`, id)
				}
			case "malformed-json":
				h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
					io.WriteString(w, `{"jsonrpc":"2.0","result":`)
				}
			case "array-result":
				h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":[]}`, id)
				}
			case "http":
				h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
					http.Error(w, "history unavailable", 503)
				}
			case "rpc":
				h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32002,"message":"program failure","data":{"err":{"InstructionError":[0,{"Custom":6001}]},"logs":["Program test failed"]}}}`, id)
				}
			case "cancel":
				h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) { cancel(); <-r.Context().Done() }
			case "deadline":
				var timeoutCancel context.CancelFunc
				ctx, timeoutCancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer timeoutCancel()
				h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) { <-r.Context().Done() }
			case "genesis-entry":
				h.genesisFlipAt = 1
			case "genesis-after-receipt":
				h.genesisFlipAt = 2
			case "genesis-outage":
				h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
					h.failMethod = "getGenesisHash"
					json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": h.receipt})
				}
			}
			err := assertHistoryRetained(t, svc, h, ctx)
			switch mode {
			case "null":
				want := fmt.Sprintf("%s: expired transaction history unresolved (%s); durable intent retained; restore signature history and transaction metadata", h.doc.Stage, h.doc.Signature)
				if err.Error() != want {
					t.Fatalf("original expiry error changed: %v", err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case "deadline":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
			case "rpc":
				var rpcErr *jsonrpc.RPCError
				if !errors.As(err, &rpcErr) || rpcErr.Code != -32002 || !strings.Contains(err.Error(), "InstructionError") || !strings.Contains(err.Error(), "6001") {
					t.Fatalf("RPC program payload lost: %v", err)
				}
			case "genesis-entry", "genesis-after-receipt":
				if !strings.Contains(err.Error(), "genesis changed") {
					t.Fatal(err)
				}
			}
			wantLookups := 1
			if mode == "genesis-entry" {
				wantLookups = 0
			}
			if h.calls["getTransaction"] != wantLookups {
				t.Fatalf("wrong lookup count: %v", h.calls)
			}
		})
	}
}

func TestHistoryRecoveryObserverCrashAndChargeDeduplication(t *testing.T) {
	for _, mode := range []string{"before-observer", "atomic-write", "after-charge", "unlink"} {
		t.Run(mode, func(t *testing.T) {
			svc, h := newHistoryRecovery(t, "proof-close", json.RawMessage(`null`), -25000)
			id, _ := native.ParseID(h.identity.ID)
			funded, reclaimed := int64(50000), int64(10000)
			_, err := svc.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
				ledger.set(id, spendEntry{
					SpentLamports:           17,
					Attempts:                4,
					BufferFundedLamports:    &funded,
					BufferReclaimedLamports: &reclaimed,
					Retry: &retryState{
						Failures: 2,
						RetryAt:  1700000000000,
					},
				})
				return true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(h.path)
			if err != nil {
				t.Fatal(err)
			}
			ledgerBefore, err := os.ReadFile(svc.store.ledgerPath)
			if err != nil {
				t.Fatal(err)
			}
			crash := errors.New("synthetic observer crash")
			blocked := filepath.Join(durableTestDirectory(t), "blocked-rename")
			if err := os.Mkdir(blocked, 0o700); err != nil {
				t.Fatal(err)
			}
			if mode == "atomic-write" {
				svc.store.writeJSON = func(path string, data []byte) error {
					if path != svc.store.ledgerPath {
						t.Error("unexpected durable write")
					}
					return atomicJSON(blocked, data)
				}
			}
			observed := 0
			err = runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.resume(context.Background(), h.path, func(result txResult) error {
					observed++
					acquired, lockErr := svc.withPayer(context.Background(), func(*payerSession) error {
						t.Error("observer ran without the payer lock")
						return nil
					})
					if acquired || lockErr != nil {
						t.Error("payer lock was not held")
					}
					if mode == "before-observer" {
						return crash
					}
					if err := p.chargeResult(context.Background(), id, result); err != nil {
						return err
					}
					if mode == "unlink" {
						if err := os.Rename(h.path, h.path+".saved"); err != nil {
							return err
						}
						if err := os.Mkdir(h.path, 0o700); err != nil {
							return err
						}
						return os.WriteFile(filepath.Join(h.path, "obstruct-unlink"), []byte("test"), 0o600)
					}
					return crash
				})
				return err
			})
			if err == nil || observed != 1 {
				t.Fatalf("fault not exposed: %v observed=%d", err, observed)
			}
			if mode == "unlink" {
				if err := os.Remove(filepath.Join(h.path, "obstruct-unlink")); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(h.path); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(h.path+".saved", h.path); err != nil {
					t.Fatal(err)
				}
			}
			after, readErr := os.ReadFile(h.path)
			if readErr != nil || !bytes.Equal(original, after) {
				t.Fatal("observer failure lost original signed intent")
			}
			if mode == "before-observer" || mode == "atomic-write" {
				ledgerAfter, readErr := os.ReadFile(svc.store.ledgerPath)
				if readErr != nil || !bytes.Equal(ledgerBefore, ledgerAfter) {
					t.Fatal("failed observer changed ledger")
				}
			}
			files, err := os.ReadDir(filepath.Dir(blocked))
			if err != nil || len(files) != 1 {
				t.Fatal("failed atomic write leaked a temporary file")
			}
			svc.store.writeJSON = atomicJSON
			if err := runDurablePayer(t, svc, func(p *payerSession) error { return p.resumePayment(context.Background(), id) }); err != nil {
				t.Fatal(err)
			}
			ledger, err := svc.store.readLedger()
			if err != nil {
				t.Fatal(err)
			}
			entry := ledger.entries[id]
			if entry.SpentLamports != 5017 || int64Value(entry.BufferFundedLamports) != 50000 || int64Value(entry.BufferReclaimedLamports) != 40000 {
				t.Fatal("restart double-charged fee or rent")
			}
			if len(entry.Signatures) != 1 || entry.Signatures[0] != h.doc.Signature {
				t.Fatal("restart changed the charged signature")
			}
			if entry.Attempts != 4 || entry.Retry == nil || entry.Retry.Failures != 2 || entry.Retry.RetryAt != 1700000000000 {
				t.Fatal("restart reset prior attempts or retry state")
			}
			if _, err := os.Stat(h.path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("retry did not unlink observed intent")
			}
			if h.calls["getTransaction"] != 2 || h.calls["sendTransaction"] != 0 {
				t.Fatalf("crash recovery retried/broadcast: %v", h.calls)
			}
		})
	}
}

func TestHistoryRecoveryNonexpiredAndEqualityPreserveReplay(t *testing.T) {
	for _, height := range []uint64{999, 1000} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			svc, h, _ := durablePaymentService(t)
			original := durableFixture(t, "ts-intent.json")
			writeFixture(t, h.path, original)
			h.blockHeight = height
			h.lostResponse = true
			err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.resume(context.Background(), h.path, func(txResult) error { t.Error("lost response observed"); return nil })
				return err
			})
			if err == nil || h.calls["getTransaction"] != 0 || h.calls["sendTransaction"] != 1 {
				t.Fatalf("nonexpired replay changed: %v %v", err, h.calls)
			}
			var doc intent
			if err := json.Unmarshal(original, &doc); err != nil {
				t.Fatal(err)
			}
			if len(h.broadcasts) != 1 || h.broadcasts[0] != doc.Raw {
				t.Fatal("nonexpired replay did not use original bytes")
			}
			after, err := os.ReadFile(h.path)
			if err != nil || !bytes.Equal(after, original) {
				t.Fatal("nonexpired intent changed")
			}
		})
	}
}

func TestHistoryRecoveryOrdinaryMetadataParity(t *testing.T) {
	for _, stage := range []string{"signature", "proof-append", "assembly", "proof-close"} {
		for _, failure := range []string{`null`, `{"InstructionError":[0,"InvalidArgument"]}`} {
			t.Run(stage+"/"+failure, func(t *testing.T) {
				svc, h, _ := durablePaymentService(t)
				var doc intent
				if err := json.Unmarshal(durableFixture(t, "ts-intent.json"), &doc); err != nil {
					t.Fatal(err)
				}
				doc.Stage = stage
				if err := svc.store.saveIntent(h.path, &doc); err != nil {
					t.Fatal(err)
				}
				h.landed, h.failure, h.delta, h.nullMetadata = true, json.RawMessage(failure), -25000, 2
				if !finalizedStage(stage) {
					h.status = "confirmed"
				}
				var normal txResult
				if err := runDurablePayer(t, svc, func(p *payerSession) error {
					_, err := p.resume(context.Background(), h.path, func(result txResult) error { normal = result; return nil })
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if h.calls["getTransaction"] != 3 || h.calls["getGenesisHash"] != 1 || h.calls["getBlockHeight"] != 0 {
					t.Fatalf("normal retry/genesis behavior changed: %v", h.calls)
				}
				svc, history := newHistoryRecovery(t, stage, json.RawMessage(failure), -25000)
				if err := runDurablePayer(t, svc, func(p *payerSession) error {
					_, err := p.resume(context.Background(), history.path, func(result txResult) error {
						if !reflect.DeepEqual(normal, result) {
							t.Error("history and normal metadata differ")
						}
						return nil
					})
					return err
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func saveHistoryTransaction(t *testing.T, svc *service, h *historyRPC, tx *solana.Transaction, sign bool) {
	t.Helper()
	if sign {
		tx.Signatures = nil
		if _, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
			if key == svc.payer.PublicKey() {
				return &svc.payer
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	h.doc.Raw = base64.StdEncoding.EncodeToString(raw)
	h.doc.Signature = tx.Signatures[0].String()
	h.doc.Accounts = len(tx.Message.AccountKeys) + tx.Message.AddressTableLookups.NumLookups()
	if err := svc.store.saveIntent(h.path, &h.doc); err != nil {
		t.Fatal(err)
	}
	h.receipt["transaction"] = []string{h.doc.Raw, "base64"}
	pre, post := make([]int64, h.doc.Accounts), make([]int64, h.doc.Accounts)
	pre[0], post[0] = 10000000, 9995000
	meta := h.receipt["meta"].(map[string]any)
	meta["preBalances"], meta["postBalances"] = pre, post
}

func TestHistoryRecoveryTransactionAndAddressIndexing(t *testing.T) {
	for _, mode := range []string{"valid-alt", "missing-loaded", "swapped-loaded", "null-loaded", "malformed-loaded", "truncated-balances", "account-index", "program-index", "forged-signature", "wrong-account-count"} {
		t.Run(mode, func(t *testing.T) {
			svc, h := newHistoryRecovery(t, "assembly", json.RawMessage(`null`), 5000)
			table, destination, readonly := solana.PublicKey{5}, solana.PublicKey{6}, solana.PublicKey{7}
			tx, _, err := bridge.Compile(svc.payer.PublicKey(), solana.Hash{8}, []solana.Instruction{
				solana.NewInstruction(solana.SystemProgramID, solana.AccountMetaSlice{
					solana.Meta(svc.payer.PublicKey()).SIGNER().WRITE(),
					solana.Meta(destination).WRITE(),
					solana.Meta(readonly),
				}, []byte{1}),
			}, map[solana.PublicKey]solana.PublicKeySlice{table: {destination, readonly}})
			if err != nil {
				t.Fatal(err)
			}
			if len(tx.Message.AddressTableLookups) != 1 || len(tx.Message.AddressTableLookups[0].WritableIndexes) != 1 || len(tx.Message.AddressTableLookups[0].ReadonlyIndexes) != 1 {
				t.Fatal("fixture did not compile real ALT indexes")
			}
			if mode == "account-index" {
				tx.Message.Instructions[0].Accounts[0] = 64
			}
			if mode == "program-index" {
				tx.Message.Instructions[0].ProgramIDIndex = 64
			}
			saveHistoryTransaction(t, svc, h, tx, true)
			meta := h.receipt["meta"].(map[string]any)
			loaded := map[string]any{
				"writable": []string{destination.String()},
				"readonly": []string{readonly.String()},
			}
			meta["loadedAddresses"] = loaded
			switch mode {
			case "missing-loaded":
				delete(meta, "loadedAddresses")
			case "swapped-loaded":
				loaded["writable"], loaded["readonly"] = []string{}, []string{destination.String(), readonly.String()}
			case "null-loaded":
				loaded["readonly"] = []any{nil}
			case "malformed-loaded":
				loaded["readonly"] = []string{"wrong-key"}
			case "truncated-balances":
				meta["postBalances"] = []int64{9995000, 0}
			case "forged-signature":
				tx.Signatures[0][0] ^= 1
				saveHistoryTransaction(t, svc, h, tx, false)
			case "wrong-account-count":
				h.doc.Accounts++
				if err := svc.store.saveIntent(h.path, &h.doc); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "valid-alt" {
				err := assertHistoryRetained(t, svc, h, context.Background())
				if !strings.Contains(err.Error(), "invalid finalized transaction receipt") {
					t.Fatal(err)
				}
				return
			}
			if err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.resume(context.Background(), h.path, func(result txResult) error {
					if result.accounts != 4 || result.fee != 5000 || *result.payerLamports != 5000 {
						t.Error("ALT account/payer indexing lost")
					}
					return nil
				})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if h.calls["sendTransaction"] != 0 || h.calls["getAccountInfo"] != 0 {
				t.Fatal("history used current accounts or broadcast")
			}
		})
	}
}

func TestHistoryRecoveryOriginalIdentityAndWire(t *testing.T) {
	for _, mode := range []string{"wrong-payer", "wrong-signature", "trailing-wire", "wire-version", "readonly-payer", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			svc, h := newHistoryRecovery(t, "signature", json.RawMessage(`null`), 5000)
			raw, _, err := h.doc.transaction()
			if err != nil {
				t.Fatal(err)
			}
			tx, err := solana.TransactionFromBytes(raw)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "wrong-payer":
				h.doc.Payer = h.doc.Genesis
			case "wrong-signature":
				h.doc.Signature = solana.Signature{1}.String()
			case "trailing-wire":
				h.doc.Raw = base64.StdEncoding.EncodeToString(append(raw, 0))
			case "wire-version":
				decoder := bin.NewBinDecoder(raw)
				count, err := decoder.ReadCompactU16()
				if err != nil {
					t.Fatal(err)
				}
				if err := decoder.SkipBytes(uint(count * solana.SignatureLength)); err != nil {
					t.Fatal(err)
				}
				raw[decoder.Position()] = 0x81
				h.doc.Raw = base64.StdEncoding.EncodeToString(raw)
			case "readonly-payer":
				tx.Message.Header.NumReadonlySignedAccounts = tx.Message.Header.NumRequiredSignatures
				saveHistoryTransaction(t, svc, h, tx, true)
			case "legacy":
				tx.Message.SetVersion(solana.MessageVersionLegacy)
				saveHistoryTransaction(t, svc, h, tx, true)
				h.receipt["version"] = "legacy"
			}
			data, err := json.Marshal(h.doc)
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, h.path, data)
			h.receipt["transaction"] = []string{h.doc.Raw, "base64"}
			if mode == "legacy" {
				if err := runDurablePayer(t, svc, func(p *payerSession) error {
					_, err := p.resume(context.Background(), h.path, func(txResult) error { return nil })
					return err
				}); err != nil {
					t.Fatal(err)
				}
				return
			}
			err = assertHistoryRetained(t, svc, h, context.Background())
			if mode == "wrong-payer" || mode == "wrong-signature" {
				if len(h.calls) != 0 {
					t.Fatal("invalid original identity reached RPC")
				}
			} else if !strings.Contains(err.Error(), "invalid finalized transaction receipt") {
				t.Fatal(err)
			}
		})
	}
}

func TestHistoryRecoveryAssemblyLogsAndOptionalCompute(t *testing.T) {
	for _, compute := range []string{"absent", "null", "zero"} {
		t.Run(compute, func(t *testing.T) {
			svc, h := newHistoryRecovery(t, "assembly", json.RawMessage(`{"InstructionError":[0,"ComputationalBudgetExceeded"]}`), 5000)
			program := solana.MustPublicKeyFromBase58(h.identity.ProgramID)
			meta := h.receipt["meta"].(map[string]any)
			logs := []string{"Program " + program.String() + " failed: Computational budget exceeded"}
			meta["logMessages"] = logs
			switch compute {
			case "absent":
				delete(meta, "computeUnitsConsumed")
			case "null":
				meta["computeUnitsConsumed"] = nil
			case "zero":
				meta["computeUnitsConsumed"] = 0
			}
			if err := runDurablePayer(t, svc, func(p *payerSession) error {
				_, err := p.resume(context.Background(), h.path, func(result txResult) error {
					if !transactionFailed(result) || result.compute != 0 || !reflect.DeepEqual(result.logs, logs) || assemblyExhaustion(result, program) != "assembly exhausted the compute budget" {
						t.Error("assembly failure semantics were lost")
					}
					return nil
				})
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHistoryRecoveryEndpointPrivacy(t *testing.T) {
	svc, h := newHistoryRecovery(t, "proof-append", json.RawMessage(`null`), 5000)
	h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
		http.Error(w, "synthetic transport outage", http.StatusServiceUnavailable)
	}
	server := httptest.NewServer(http.HandlerFunc(h.serveHistory))
	t.Cleanup(server.Close)
	privateEndpoint := strings.Replace(server.URL, "http://", "http://synthetic-user:synthetic-password@", 1) + "/synthetic-private-path?api-key=synthetic-token"
	endpoint, client, err := native.RPCEndpointHTTP(privateEndpoint, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	svc.solana = rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts(endpoint, &jsonrpc.RPCClientOpts{
		HTTPClient: client,
	}))
	err = assertHistoryRetained(t, svc, h, context.Background())
	for _, secret := range []string{"synthetic-user", "synthetic-password", "synthetic-private-path", "synthetic-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Error("history error exposed synthetic endpoint credentials")
		}
	}
	if h.calls["getTransaction"] != 1 {
		t.Fatal("history did not use configured primary transport")
	}
}

func TestHistoryRecoveryMalformedEnvelopeRetainsOriginal(t *testing.T) {
	for _, mode := range []string{"null-envelope", "missing-protocol", "wrong-protocol", "missing-id", "wrong-id", "conflicting-result", "malformed-error", "missing-code", "null-code", "string-code", "missing-message", "null-message", "trailing-value", "http-success-body"} {
		t.Run(mode, func(t *testing.T) {
			svc, h := newHistoryRecovery(t, "proof-append", json.RawMessage(`null`), 5000)
			h.reply = func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
				envelope := map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result":  h.receipt,
				}
				failure := map[string]any{
					"code":    -32002,
					"message": "synthetic rejection",
				}
				switch mode {
				case "null-envelope":
					envelope = nil
				case "missing-protocol":
					delete(envelope, "jsonrpc")
				case "wrong-protocol":
					envelope["jsonrpc"] = "1.0"
				case "missing-id":
					delete(envelope, "id")
				case "wrong-id":
					envelope["id"] = "different-request"
				case "conflicting-result":
					envelope["error"] = failure
				case "malformed-error":
					delete(envelope, "result")
					envelope["error"] = false
				case "missing-code", "null-code", "string-code", "missing-message", "null-message":
					delete(envelope, "result")
					envelope["error"] = failure
					switch mode {
					case "missing-code":
						delete(failure, "code")
					case "null-code":
						failure["code"] = nil
					case "string-code":
						failure["code"] = "-32002"
					case "missing-message":
						delete(failure, "message")
					case "null-message":
						failure["message"] = nil
					}
				case "http-success-body":
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				if err := json.NewEncoder(w).Encode(envelope); err != nil {
					t.Error(err)
				}
				if mode == "trailing-value" {
					io.WriteString(w, `{}`)
				}
			}
			err := assertHistoryRetained(t, svc, h, context.Background())
			if !strings.Contains(err.Error(), "finalized transaction history lookup") || h.calls["getTransaction"] != 1 {
				t.Fatalf("envelope rejection lost: %v", err)
			}
		})
	}
}

func TestHistoryRecoveryExpiryReadFailureDoesNotLookup(t *testing.T) {
	svc, h := newHistoryRecovery(t, "signature", json.RawMessage(`null`), 5000)
	h.failMethod = "getBlockHeight"
	assertHistoryRetained(t, svc, h, context.Background())
	if h.calls["getTransaction"] != 0 || h.calls["getBlockHeight"] != 1 {
		t.Fatal("history lookup ran without observed expiry")
	}
}
