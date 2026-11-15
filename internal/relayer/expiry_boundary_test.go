package relayer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	solana "github.com/gagliardetto/solana-go"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func TestExpiryCloseAbsenceGuardAndFreshNextPass(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	f.service.reader = bridge.NewReader(f.service.solana, f.service.cfg.program, f.service.cfg.chainID, nil)
	original, _ := os.ReadFile(f.path + ".proof")
	closeBuffer := func(p *payerSession) error { return p.closeProofBuffer(context.Background(), f.id) }
	err := runDurablePayer(t, f.service, closeBuffer)
	if !isIntentNotIncluded(err) || f.calls["getAccountInfo"] != 0 {
		t.Fatalf("cleanup bypassed unresolved pending: %v", err)
	}
	retained, _ := os.ReadFile(f.path + ".proof")
	if !bytes.Equal(original, retained) || f.observed != 0 {
		t.Fatal("certification entered cleanup or accounting")
	}
	if err := runDurablePayer(t, f.service, closeBuffer); err != nil {
		t.Fatal(err)
	}
	if f.calls["getAccountInfo"] != 1 || f.calls["sendTransaction"] != 0 {
		t.Fatal("next pass did not re-read buffer absence")
	}
	if _, err := os.Stat(f.path + ".proof"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("ordinary absent-buffer cleanup did not resume")
	}
}

func TestExpiryPaidUnwindsPrebuiltInstructions(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	if err := runDurablePayer(t, f.service, func(p *payerSession) error {
		_, err := p.paid(context.Background(), f.id, "resolve", nil, false)
		return err
	}); !isIntentNotIncluded(err) {
		t.Fatal(err)
	}
	if f.calls["getLatestBlockhash"] != 0 || f.calls["simulateTransaction"] != 0 || f.calls["sendTransaction"] != 0 {
		t.Fatal("stale prebuilt work continued after certification")
	}
	ready, err := f.service.headerStep(func() error { return nil }, 10)
	if !ready || err != nil {
		t.Fatal("normal next step was deferred by expiry retry bookkeeping")
	}
}

func TestExpiryOverBudgetMarkerEscapes(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	ledger, _ := f.service.store.readLedger()
	if err := ledger.charge(f.id, 20000, "earlier", 0); err != nil {
		t.Fatal(err)
	}
	before := ledger.document(f.service.genesis, f.service.store.identity.ProgramID)
	err := f.service.overBudget(context.Background(), f.id, ledger)
	if !isIntentNotIncluded(err) || !reflect.DeepEqual(before, ledger.document(f.service.genesis, f.service.store.identity.ProgramID)) {
		t.Fatalf("over-budget swallowed marker or changed ledger: %v", err)
	}
}

func TestExpirySendLeavesNewPendingForNextPass(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	raw, _ := base64.StdEncoding.DecodeString(f.doc.Raw)
	tx, _ := solana.TransactionFromBytes(raw)
	ix := solana.NewInstruction(tx.Message.AccountKeys[tx.Message.Instructions[0].ProgramIDIndex], solana.AccountMetaSlice{
		solana.Meta(f.service.payer.PublicKey()).SIGNER(),
		solana.Meta(tx.Message.AccountKeys[1]).WRITE(),
	}, tx.Message.Instructions[0].Data)
	if err := os.Remove(f.path); err != nil {
		t.Fatal(err)
	}
	err := runDurablePayer(t, f.service, func(p *payerSession) error {
		_, err := p.send(context.Background(), txRequest{
			stage:        "proof-append",
			instructions: []solana.Instruction{ix},
		}, f.path, func(txResult) error {
			t.Error("expired send invoked observer")
			return nil
		})
		return err
	})
	var expired *expiredHistoryUnavailable
	if !errors.As(err, &expired) || isIntentNotIncluded(err) {
		t.Fatalf("send certified or changed expired result: %v", err)
	}
	if _, err := os.Stat(f.path); err != nil || f.calls["getBlock"] != 0 || f.calls["sendTransaction"] != 0 || f.calls["getLatestBlockhash"] != 1 {
		t.Fatal("send retired or retried the newly saved intent")
	}
}

func TestExpiryLegacyAndALTWithoutLiveLookup(t *testing.T) {
	for _, version := range []string{"legacy", "v0-alt"} {
		t.Run(version, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			raw, _ := base64.StdEncoding.DecodeString(f.doc.Raw)
			tx, _ := solana.TransactionFromBytes(raw)
			if version == "legacy" {
				tx.Message.SetVersion(solana.MessageVersionLegacy)
			} else {
				tx.Message.AddressTableLookups = []solana.MessageAddressTableLookup{
					{
						AccountKey:      f.service.cfg.program,
						WritableIndexes: []uint8{255},
						ReadonlyIndexes: []uint8{3},
					},
				}
				f.doc.Accounts += 2
				tx.Message.Instructions[0].Accounts = append(tx.Message.Instructions[0].Accounts, uint16(f.doc.Accounts-1))
			}
			f.saveTransaction(tx)
			if err := f.resume(context.Background()); !isIntentNotIncluded(err) {
				t.Fatal(err)
			}
			if f.calls["getAccountInfo"] != 0 {
				t.Fatal("used present lookup contents for historical absence")
			}
		})
	}
}

func TestExpiryRPCBudgetsAndCancellation(t *testing.T) {
	for _, limit := range []string{"requests", "overall-time", "aggregate-size", "response-size", "canceled", "request-deadline"} {
		t.Run(limit, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			r := expiryRPC{
				service: f.service,
				started: f.service.deps.now(),
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch limit {
			case "requests":
				r.calls = expiryMaxRequests
			case "overall-time":
				r.started = r.started.Add(-expiryTimeout)
			case "aggregate-size":
				r.bytes = expiryMaxTotal - 1
			case "response-size":
				f.reply = func(w http.ResponseWriter, _ json.RawMessage, _ string) bool {
					w.Write(bytes.Repeat([]byte(" "), expiryMaxResponse+1))
					return true
				}
			case "canceled":
				cancel()
			case "request-deadline":
				var end context.CancelFunc
				ctx, end = context.WithDeadline(ctx, time.Unix(1, 0))
				defer end()
			}
			if _, err := r.call(ctx, "getGenesisHash", nil); err == nil {
				t.Fatal("RPC budget ignored")
			}
			if (limit == "requests" || limit == "overall-time" || limit == "canceled" || limit == "request-deadline") && len(f.calls) != 0 {
				t.Fatal("exhausted operation reached transport")
			}
		})
	}
}

func TestExpiryRequestSpacingAndRateLimitDelay(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	r := expiryRPC{
		service: f.service,
		started: f.service.deps.now(),
	}
	for i := 0; i < 3; i++ {
		if _, err := r.call(context.Background(), "getGenesisHash", nil); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i < len(f.times); i++ {
		if f.times[i].Sub(f.times[i-1]) < expiryRequestGap {
			t.Fatal("RPC pacing floor ignored")
		}
	}
	f.reply = func(w http.ResponseWriter, id json.RawMessage, method string) bool {
		if method != "getBlock" {
			return false
		}
		w.Header().Set("Retry-After", "7200")
		w.WriteHeader(429)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":429,"message":"limited"}}`, id)
		return true
	}
	f.retained(context.Background())
	saved, _ := f.service.store.snapshotPending(f.path)
	cooldown := f.service.expiryCooldowns[saved.Digest]
	if cooldown.next.Sub(f.service.deps.now()) < 2*time.Hour {
		t.Fatal("longer provider cooldown was lost")
	}
}

func TestExpiryBoundedLocatorAndAnchorPolicy(t *testing.T) {
	for _, mode := range []string{"anchor-at-end", "anchor-before-expiry", "equality", "empty-locator", "omitted-locator-slot", "unsorted-locator", "outside-bracket"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			saved, _ := f.service.store.snapshotPending(f.path)
			switch mode {
			case "anchor-at-end":
				f.anchor = 1002
			case "anchor-before-expiry":
				f.anchor = 999
			case "equality":
				f.anchor = 1000
			case "outside-bracket":
				f.anchor = 100000
			}
			f.mutate = func(method string, _ []json.RawMessage, result any) any {
				if method != "getBlocks" {
					return result
				}
				slots := result.([]uint64)
				switch mode {
				case "empty-locator":
					return []uint64{}
				case "omitted-locator-slot":
					out := []uint64{}
					for _, slot := range slots {
						if slot != expiryTestSlot(1002) {
							out = append(out, slot)
						}
					}
					return out
				case "unsorted-locator":
					if len(slots) > 1 {
						slots[0], slots[1] = slots[1], slots[0]
					}
				}
				return slots
			}
			err := runDurablePayer(t, f.service, func(p *payerSession) error {
				_, err := p.certifyExpired(context.Background(), saved)
				return err
			})
			if mode == "anchor-at-end" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("unsupported locator or anchor accepted")
			}
			if f.calls["getBlocks"] > 18 || f.observed != 0 || f.calls["sendTransaction"] != 0 {
				t.Fatal("locator exceeded bounds or caused effects")
			}
		})
	}
}

func TestExpirySnapshotIdentityAndPaths(t *testing.T) {
	for _, mode := range []string{"wrong-filename", "wrong-proof-payer", "wrong-proof-key", "wrong-stage", "missing-proof", "pending-symlink", "ancestor-symlink", "unknown-path"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			s := f.service.store
			path := f.path
			switch mode {
			case "wrong-filename", "unknown-path":
				path = filepath.Join(filepath.Dir(f.path), "wrong.json")
				if mode == "unknown-path" {
					path = filepath.Join(s.expiryRoot(), "wrong.json")
				}
				data, _ := os.ReadFile(f.path)
				os.WriteFile(path, data, 0o600)
				proof, _ := os.ReadFile(f.path + ".proof")
				os.WriteFile(path+".proof", proof, 0o600)
			case "wrong-proof-payer", "wrong-proof-key":
				data, _ := os.ReadFile(f.path + ".proof")
				var fields map[string]any
				json.Unmarshal(data, &fields)
				field := "payer"
				if mode == "wrong-proof-key" {
					field = "key"
				}
				fields[field] = "bad"
				data, _ = json.Marshal(fields)
				os.WriteFile(f.path+".proof", data, 0o600)
			case "wrong-stage":
				f.doc.Stage = "signature"
				s.saveIntent(f.path, &f.doc)
			case "missing-proof":
				os.Remove(f.path + ".proof")
			case "pending-symlink":
				os.Remove(f.path)
				os.Symlink(f.path+".proof", f.path)
			case "ancestor-symlink":
				moved := s.jobDirectory + "-moved"
				os.Rename(s.jobDirectory, moved)
				os.Symlink(moved, s.jobDirectory)
			}
			if _, err := s.snapshotPending(path); err == nil {
				t.Fatal("unowned or malformed pending accepted")
			}
		})
	}
}

func TestExpiryWatcherRebuildsOnNextTick(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	f.service.reader = bridge.NewReader(f.service.solana, f.service.cfg.program, f.service.cfg.chainID, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := 0
	err := runTicks(ctx, true, 3*time.Second, func(ctx context.Context) error {
		ticks++
		err := runDurablePayer(t, f.service, func(p *payerSession) error {
			return p.closeProofBuffer(ctx, f.id)
		})
		if ticks == 1 {
			if !isIntentNotIncluded(err) || f.calls["getAccountInfo"] != 0 {
				t.Fatal("first tick continued after certified marker")
			}
			return err
		}
		if ticks != 2 || err != nil || f.calls["getAccountInfo"] != 1 {
			t.Fatal("second tick did not rebuild cleanup from fresh state")
		}
		cancel()
		return nil
	}, f.service.deps)
	if err != nil || ticks != 2 {
		t.Fatalf("watch loop did not resume normally: %v", err)
	}
}

func TestExpiryCachedProofRequiresCurrentChecks(t *testing.T) {
	for _, mode := range []string{"positive-status", "positive-receipt", "wrong-genesis", "regressed-anchor", "valid-blockhash", "cancel", "anchor-inclusion", "anchor-conflict"} {
		t.Run(mode, func(t *testing.T) {
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
			before := make(map[string]int)
			for method, count := range f.calls {
				before[method] = count
			}
			f.mutate = func(method string, _ []json.RawMessage, result any) any {
				calls := f.calls[method] - before[method]
				switch mode {
				case "positive-status":
					if method == "getSignatureStatuses" && calls == 3 {
						result.(map[string]any)["value"] = []any{map[string]any{"slot": 3000}}
					}
				case "positive-receipt":
					if method == "getTransaction" && calls == 2 {
						return map[string]any{"slot": 3000}
					}
				case "wrong-genesis":
					if method == "getGenesisHash" && calls == 2 {
						return expiryTestHash(44).String()
					}
				case "regressed-anchor":
					if method == "getSlot" {
						return expiryTestSlot(1009)
					}
				case "anchor-inclusion":
					if method == "getBlock" {
						result.(map[string]any)["signatures"] = []string{f.doc.Signature}
					}
				case "anchor-conflict":
					if method == "getBlock" {
						result.(map[string]any)["signatures"] = []string{solana.Signature{1}.String()}
					}
				case "valid-blockhash":
					if method == "isBlockhashValid" {
						result.(map[string]any)["value"] = true
					}
				}
				return result
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			f.retained(ctx)
			if f.calls["getBlocks"] != before["getBlocks"] || f.calls["getBlock"]-before["getBlock"] > 1 {
				t.Fatal("cached proof rescanned history")
			}
		})
	}
}

func TestExpiryCapsulePureValidationAndCollision(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	saved, _ := f.service.store.snapshotPending(f.path)
	var original *nonInclusionCertificate
	if err := runDurablePayer(t, f.service, func(p *payerSession) error {
		var err error
		original, err = p.certifyExpired(context.Background(), saved)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"version", "policy", "age", "missing-block", "first-block", "origin-only-previous", "locator", "status", "receipt", "hash-valid", "context", "wrong-digest", "positive-signature", "anchor-conflict"} {
		t.Run(mode, func(t *testing.T) {
			data, _ := json.Marshal(original)
			var cert nonInclusionCertificate
			json.Unmarshal(data, &cert)
			switch mode {
			case "version":
				cert.Version++
			case "policy":
				cert.Policy = "other"
			case "age":
				cert.MaxAge++
			case "missing-block":
				cert.Blocks = cert.Blocks[:164]
			case "first-block":
				cert.Blocks[164].Blockhash = expiryTestHash(1).String()
			case "origin-only-previous":
				cert.Blocks[152].Blockhash = expiryTestHash(1).String()
			case "locator":
				cert.Locator[0].Start++
			case "status":
				cert.Final.StatusContext = 0
			case "receipt":
				cert.Final.ReceiptAbsent = false
			case "hash-valid":
				cert.Final.BlockhashValid = true
			case "context":
				cert.Final.HashContext = 0
			case "wrong-digest":
				cert.PendingDigest = "wrong"
			case "positive-signature":
				cert.Blocks[100].Signatures = []string{f.doc.Signature}
			case "anchor-conflict":
				cert.Final.Anchor.Signatures = []string{solana.Signature{1}.String()}
			}
			if err := validateExpiryCertificate(saved, &cert); err == nil {
				t.Fatal("corrupt full certificate authorized reuse")
			}
		})
	}
	if err := f.service.store.publishExpiry(saved, original, expiryFiles()); err != nil {
		t.Fatal(err)
	}
	copy := *original
	copy.Finished++
	if err := f.service.store.publishExpiry(saved, &copy, expiryFiles()); err == nil {
		t.Fatal("immutable capsule overwritten")
	}
}

func TestExpiryHeaderResumeStopsBeforeDeferredCleanup(t *testing.T) {
	f := newExpiryFixture(t, "signature")
	h := newHeaderJobFixture(t)
	layout, reason, err := layoutHeader(h.f, f.service.payer.PublicKey(), h.packet, 11, 10, 1)
	if err != nil || reason != "" {
		t.Fatalf("header layout: %v %s", err, reason)
	}
	id := hex.EncodeToString(layout.session[:])
	f.path, err = f.service.store.sessionPendingPath(id)
	if err != nil {
		t.Fatal(err)
	}
	instruction, err := h.f.PreVerifySignatureBatch(f.service.payer.PublicKey(), layout.batches[0], h.packet.Receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	tx, accounts, err := bridge.Compile(f.service.payer.PublicKey(), expiryTestHash(850), []solana.Instruction{instruction}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.doc.Accounts = accounts
	f.saveTransaction(tx)
	err = runDurablePayer(t, f.service, func(p *payerSession) error {
		_, err := p.uploadHeader(context.Background(), h.f, h.packet, 11, 10, 1)
		return err
	})
	if !isIntentNotIncluded(err) {
		t.Fatal(err)
	}
	if f.calls["getAccountInfo"] != 0 || f.calls["sendTransaction"] != 0 || f.observed != 0 {
		t.Fatal("header certification entered deferred cleanup")
	}
	if _, err := os.Stat(filepath.Join(f.service.store.headerDirectory, "transactions.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("header certification appended an observer event")
	}
}

func TestExpiryCertificationHoldsPayerLock(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	checked := false
	f.mutate = func(method string, _ []json.RawMessage, value any) any {
		if method == "isBlockhashValid" {
			checked = true
			acquired, err := f.service.withPayer(context.Background(), func(*payerSession) error {
				t.Error("second payer entered certification lock")
				return nil
			})
			if acquired || err != nil {
				t.Error("certification lost the existing payer lock")
			}
		}
		return value
	}
	if err := f.resume(context.Background()); !isIntentNotIncluded(err) || !checked {
		t.Fatalf("lock certification fixture failed: %v", err)
	}
}
