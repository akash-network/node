package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func expiryIdentityCertificate(t *testing.T, f *expiryFixture) (pendingSnapshot, *nonInclusionCertificate) {
	t.Helper()
	saved, err := f.service.store.snapshotPending(f.path)
	if err != nil {
		t.Fatal(err)
	}
	var cert *nonInclusionCertificate
	if err := runDurablePayer(t, f.service, func(p *payerSession) error {
		var err error
		cert, err = p.certifyExpired(context.Background(), saved)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return saved, cert
}

func expiryIdentityOldestLocator(t *testing.T, cert *nonInclusionCertificate) int {
	t.Helper()
	if len(cert.Locator) == 0 {
		t.Fatal("fixture needs locator evidence")
	}
	index := 0
	for i := range cert.Locator {
		if cert.Locator[i].Block.Slot < cert.Locator[index].Block.Slot {
			index = i
		}
	}
	oldest := cert.Locator[index].Block
	for _, block := range cert.blockViews() {
		if block.Slot < oldest.Slot || block.Slot == oldest.ParentSlot || block.Slot == cert.Initial.Anchor.ParentSlot {
			t.Fatal("fixture needs earliest locator and unfetched parents")
		}
	}
	if oldest.Slot != 1510 || oldest.Height != 255 {
		t.Fatalf("unexpected earliest locator: %+v", oldest)
	}
	return index
}

// Corrupt only the certificate in an otherwise canonical, digest-matching
// capsule. This exercises loading even when normal publication refuses it.
func expiryIdentityWriteCapsule(t *testing.T, f *expiryFixture, saved pendingSnapshot, cert *nonInclusionCertificate) []byte {
	t.Helper()
	certificate, err := json.Marshal(cert)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(expiryCapsule{
		Version:         1,
		Pending:         saved,
		Certificate:     cert,
		CertificateHash: digestHex(certificate),
	})
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(f.service.store.expiryCapsulePath(saved), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExpiryIdentityContradictions(t *testing.T) {
	for _, kind := range []string{"impossible-parent", "observed-hash", "implied-observed-hash", "implied-self-hash", "implied-implied-hash", "zero-height"} {
		for _, mode := range []string{"pure", "fresh", "cached"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				f := newExpiryFixture(t, "proof-append")
				saved, cert := expiryIdentityCertificate(t, f)
				index := expiryIdentityOldestLocator(t, cert)
				if mode == "cached" {
					if err := f.service.store.publishExpiry(saved, cert, expiryFiles()); err != nil {
						t.Fatal(err)
					}
					if loaded, err := f.service.store.loadExpiryCertificate(saved); err != nil || loaded == nil {
						t.Fatalf("coherent capsule did not load: %v", err)
					}
				}
				block := &cert.Locator[index].Block
				want := "conflicting expiry blockhash identity"
				switch kind {
				case "impossible-parent":
					block.ParentSlot = 1
					block.PreviousBlockhash = expiryTestHash(777).String()
					want = "invalid expiry block identity"
				case "observed-hash":
					block.Blockhash = cert.Blocks[0].Blockhash
				case "implied-observed-hash":
					block.PreviousBlockhash = cert.Blocks[0].Blockhash
				case "implied-self-hash":
					block.PreviousBlockhash = block.Blockhash
				case "implied-implied-hash":
					block.PreviousBlockhash = cert.Initial.Anchor.PreviousBlockhash
				case "zero-height":
					block.Height = 0
					want = "conflicting expiry parent order"
				}
				if err := block.validate(); err != nil {
					t.Fatalf("explicit fixture must remain structurally valid: %v", err)
				}
				if mode == "pure" {
					if err := validateExpiryCertificate(saved, cert); err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("wrong identity rejection: %v, want %s", err, want)
					}
					return
				}
				beforeBlocks, beforeLocator := f.calls["getBlock"], f.calls["getBlocks"]
				var capsule []byte
				changed := false
				if mode == "cached" {
					capsule = expiryIdentityWriteCapsule(t, f, saved, cert)
					if loaded, err := f.service.store.loadExpiryCertificate(saved); err == nil || loaded != nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("corrupt canonical capsule loaded or failed for another reason: %v", err)
					}
				} else {
					f.mutate = func(method string, params []json.RawMessage, result any) any {
						if method == "getBlock" {
							var slot uint64
							if err := json.Unmarshal(params[0], &slot); err != nil {
								t.Error(err)
							}
							if slot == block.Slot {
								response := result.(map[string]any)
								response["blockHeight"] = block.Height
								response["parentSlot"] = block.ParentSlot
								response["blockhash"] = block.Blockhash
								response["previousBlockhash"] = block.PreviousBlockhash
								changed = true
							}
						}
						return result
					}
				}
				if err := f.retained(context.Background()); !strings.Contains(err.Error(), want) {
					t.Fatalf("wrong retention failure: %v, want %s", err, want)
				}
				if mode == "fresh" && !changed {
					t.Fatal("contradictory locator was not exercised")
				}
				if mode == "cached" {
					data, err := os.ReadFile(f.service.store.expiryCapsulePath(saved))
					if err != nil || !bytes.Equal(data, capsule) || f.calls["getBlock"] != beforeBlocks || f.calls["getBlocks"] != beforeLocator {
						t.Fatal("corrupt capsule was modified or triggered a chain scan")
					}
				}
			})
		}
	}
}

func TestExpiryIdentityControls(t *testing.T) {
	for _, emptyLocator := range []bool{false, true} {
		name := "locator"
		if emptyLocator {
			name = "anchor-at-end"
		}
		for _, mode := range []string{"pure", "fresh", "cached"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				f := newExpiryFixture(t, "proof-append")
				if emptyLocator {
					f.anchor = 1002
				}
				saved, cert := expiryIdentityCertificate(t, f)
				if (len(cert.Locator) == 0) != emptyLocator {
					t.Fatal("wrong locator control")
				}
				if mode == "pure" {
					if err := validateExpiryCertificate(saved, cert); err != nil {
						t.Fatal(err)
					}
					// Repeated full observations and repeated parent identities agree.
					views := append(cert.blockViews(), cert.blockViews()...)
					if err := validateExpiryBlockViews(views); err != nil {
						t.Fatalf("identical repeated views refused: %v", err)
					}
					return
				}
				var capsule []byte
				if mode == "cached" {
					if err := f.service.store.publishExpiry(saved, cert, expiryFiles()); err != nil {
						t.Fatal(err)
					}
					var err error
					capsule, err = os.ReadFile(f.service.store.expiryCapsulePath(saved))
					if err != nil {
						t.Fatal(err)
					}
				}
				beforeBlocks, beforeLocator := f.calls["getBlock"], f.calls["getBlocks"]
				if err := f.resume(context.Background()); !isIntentNotIncluded(err) {
					t.Fatalf("coherent retirement refused: %v", err)
				}
				if _, err := os.Stat(f.path); !errors.Is(err, os.ErrNotExist) || f.observed != 0 || f.calls["sendTransaction"] != 0 {
					t.Fatal("coherent retirement lost marker or changed send/observer behavior")
				}
				if mode == "cached" {
					data, err := os.ReadFile(f.service.store.expiryCapsulePath(saved))
					if err != nil || !bytes.Equal(data, capsule) || f.calls["getBlock"]-beforeBlocks != 1 || f.calls["getBlocks"] != beforeLocator {
						t.Fatal("cached control changed immutable evidence or scan bounds")
					}
				}
			})
		}
	}
}

func TestExpiryGenesisBoundary(t *testing.T) {
	for _, parentSlot := range []uint64{0, 1} {
		name := "genesis-parent"
		if parentSlot != 0 {
			name = "non-genesis-parent"
		}
		for _, mode := range []string{"pure", "certify", "fresh", "cached"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				f := newExpiryFixture(t, "proof-append")
				if _, err := f.service.store.updateLedger(context.Background(), func(l *spendLedger) (bool, error) {
					if err := l.attempt(f.id); err != nil {
						return false, err
					}
					return true, l.charge(f.id, 15000, "earlier-observed", 100000)
				}); err != nil {
					t.Fatal(err)
				}
				read := func(path string) []byte {
					t.Helper()
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					return data
				}
				pending, ledger, proof := read(f.path), read(f.service.store.ledgerPath), read(f.path+".proof")
				saved, cert := expiryIdentityCertificate(t, f)
				index := expiryIdentityOldestLocator(t, cert)
				if len(cert.Blocks) != expiryWindowBlocks || len(cert.Locator) != 9 || cert.Requests > expiryMaxRequests {
					t.Fatal("fixture did not exercise bounded full certification")
				}
				if mode == "cached" {
					if err := f.service.store.publishExpiry(saved, cert, expiryFiles()); err != nil {
						t.Fatal(err)
					}
					if loaded, err := f.service.store.loadExpiryCertificate(saved); err != nil || loaded == nil {
						t.Fatalf("coherent capsule did not load: %v", err)
					}
				}
				block := &cert.Locator[index].Block
				block.Height = 1
				block.ParentSlot = parentSlot
				block.PreviousBlockhash = expiryTestHash(0).String()
				if err := block.validate(); err != nil {
					t.Fatalf("explicit locator must remain structurally valid: %v", err)
				}
				if block.PreviousBlockhash == saved.doc.Genesis {
					t.Fatal("fixture must not equate genesis identity with its blockhash")
				}
				check := func(err error) {
					t.Helper()
					if parentSlot == 0 {
						if err != nil {
							t.Errorf("coherent first produced block refused: %v", err)
						}
					} else if err == nil || !strings.Contains(err.Error(), "conflicting expiry parent order") {
						t.Errorf("non-genesis height-zero parent accepted or wrong rejection: %v", err)
					}
				}
				beforeBlocks, beforeLocator := f.calls["getBlock"], f.calls["getBlocks"]
				var capsule []byte
				changed := false
				if mode == "certify" || mode == "fresh" {
					f.mutate = func(method string, params []json.RawMessage, result any) any {
						if method == "getBlock" {
							var slot uint64
							if err := json.Unmarshal(params[0], &slot); err != nil {
								t.Error(err)
							}
							if slot == block.Slot {
								response := result.(map[string]any)
								response["blockHeight"] = block.Height
								response["parentSlot"] = block.ParentSlot
								response["previousBlockhash"] = block.PreviousBlockhash
								changed = true
							}
						}
						return result
					}
				}
				switch mode {
				case "pure":
					check(validateExpiryCertificate(saved, cert))
				case "certify":
					check(runDurablePayer(t, f.service, func(p *payerSession) error {
						actual, err := p.certifyExpired(context.Background(), saved)
						if actual == nil || actual.Requests != cert.Requests || actual.Locator[index].Block.Height != 1 || actual.Locator[index].Block.ParentSlot != parentSlot {
							t.Error("actual certification did not reach the contradictory identity boundary within the original request bound")
						}
						return err
					}))
				case "cached":
					capsule = expiryIdentityWriteCapsule(t, f, saved, cert)
					loaded, err := f.service.store.loadExpiryCertificate(saved)
					check(err)
					if (loaded != nil) != (parentSlot == 0) {
						t.Error("cached loader returned the wrong certificate presence")
					}
				}
				retired := parentSlot == 0 && (mode == "fresh" || mode == "cached")
				if mode == "fresh" || mode == "cached" {
					err := f.resume(context.Background())
					if retired {
						if !isIntentNotIncluded(err) {
							t.Errorf("coherent first produced block did not retire: %v", err)
						}
					} else {
						check(err)
					}
				}
				data, err := os.ReadFile(f.path)
				if retired {
					if !errors.Is(err, os.ErrNotExist) {
						t.Error("coherent pending intent was not retired")
					}
				} else if err != nil || !bytes.Equal(data, pending) {
					t.Error("pending intent bytes were not retained")
				}
				if !bytes.Equal(read(f.service.store.ledgerPath), ledger) || !bytes.Equal(read(f.path+".proof"), proof) || f.observed != 0 || f.calls["sendTransaction"] != 0 || f.calls["simulateTransaction"] != 0 {
					t.Error("genesis boundary changed accounting, associated evidence or send/observer behavior")
				}
				wantBlocks, wantLocator := 0, 0
				if mode == "certify" || mode == "fresh" {
					wantBlocks, wantLocator = beforeBlocks, beforeLocator
					if !changed {
						t.Error("first-produced locator response was not exercised")
					}
				} else if retired {
					wantBlocks = 1
				}
				if f.calls["getBlock"]-beforeBlocks != wantBlocks || f.calls["getBlocks"]-beforeLocator != wantLocator {
					t.Errorf("unexpected scan counts: blocks=%d locator=%d", f.calls["getBlock"]-beforeBlocks, f.calls["getBlocks"]-beforeLocator)
				}
				if mode == "cached" {
					if !bytes.Equal(read(f.service.store.expiryCapsulePath(saved)), capsule) {
						t.Error("cached capsule bytes were changed")
					}
				} else if !retired {
					if _, err := os.Stat(f.service.store.expiryCapsulePath(saved)); !errors.Is(err, os.ErrNotExist) {
						t.Error("unexpected capsule publication")
					}
				}
			})
		}
	}
}

func TestExpiryLocatorLimitBeforeViews(t *testing.T) {
	for _, kind := range []string{"same-slot-conflict", "parent-order", "anchor-at-end"} {
		for _, mode := range []string{"pure", "cached"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				f := newExpiryFixture(t, "proof-append")
				saved, cert := expiryIdentityCertificate(t, f)
				if mode == "cached" {
					if err := f.service.store.publishExpiry(saved, cert, expiryFiles()); err != nil {
						t.Fatal(err)
					}
				}
				for len(cert.Locator) <= 18 {
					cert.Locator = append(cert.Locator, cert.Locator[0])
				}
				last := &cert.Locator[len(cert.Locator)-1].Block
				switch kind {
				case "same-slot-conflict":
					last.Blockhash = cert.Blocks[0].Blockhash
				case "parent-order":
					*last = cert.Final.Anchor
					last.Slot += 4
					last.Height++
					last.ParentSlot = cert.Final.Anchor.Slot - 1
					last.Blockhash = expiryTestHash(1011).String()
				case "anchor-at-end":
					cert.Initial.Anchor = cert.Blocks[0]
					last.Height = 0
				}
				if err := validateExpiryCertificate(saved, cert); err == nil || !strings.Contains(err.Error(), "locator length") {
					t.Fatalf("overlength locator reached view validation: %v", err)
				}
				if mode == "cached" {
					capsule := expiryIdentityWriteCapsule(t, f, saved, cert)
					if loaded, err := f.service.store.loadExpiryCertificate(saved); err == nil || loaded != nil || !strings.Contains(err.Error(), "locator length") {
						t.Fatalf("overlength cached locator reached view validation: %v", err)
					}
					beforeBlocks, beforeLocator := f.calls["getBlock"], f.calls["getBlocks"]
					if err := f.retained(context.Background()); !strings.Contains(err.Error(), "locator length") {
						t.Fatalf("wrong bound retention failure: %v", err)
					}
					data, err := os.ReadFile(f.service.store.expiryCapsulePath(saved))
					if err != nil || !bytes.Equal(data, capsule) || f.calls["getBlock"] != beforeBlocks || f.calls["getBlocks"] != beforeLocator {
						t.Fatal("overlength capsule was modified or triggered a scan")
					}
				}
			})
		}
	}
}

func TestExpiryCurrentAnchorHashIdentity(t *testing.T) {
	for _, kind := range []string{"observed", "implied"} {
		for _, mode := range []string{"fresh", "cached"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				f := newExpiryFixture(t, "proof-append")
				saved, cert := expiryIdentityCertificate(t, f)
				var capsule []byte
				if mode == "cached" {
					if err := f.service.store.publishExpiry(saved, cert, expiryFiles()); err != nil {
						t.Fatal(err)
					}
					var err error
					capsule, err = os.ReadFile(f.service.store.expiryCapsulePath(saved))
					if err != nil {
						t.Fatal(err)
					}
				}
				beforeSlots, beforeBlocks, beforeLocator := f.calls["getSlot"], f.calls["getBlock"], f.calls["getBlocks"]
				changed := false
				f.mutate = func(method string, _ []json.RawMessage, result any) any {
					if method == "getSlot" && (mode == "cached" || f.calls["getSlot"]-beforeSlots == 2) {
						return expiryTestSlot(1012)
					}
					if method == "getBlock" {
						block := result.(map[string]any)
						if block["blockHeight"] == uint64(1012) {
							field := "blockhash"
							if kind == "implied" {
								field = "previousBlockhash"
							}
							block[field] = cert.Blocks[0].Blockhash
							changed = true
						}
					}
					return result
				}
				if err := f.retained(context.Background()); !strings.Contains(err.Error(), "conflicting expiry blockhash identity") {
					t.Fatalf("wrong current-anchor identity failure: %v", err)
				}
				if !changed {
					t.Fatal("current anchor contradiction was not exercised")
				}
				if mode == "cached" {
					data, err := os.ReadFile(f.service.store.expiryCapsulePath(saved))
					if err != nil || !bytes.Equal(data, capsule) || f.calls["getBlock"]-beforeBlocks != 1 || f.calls["getBlocks"] != beforeLocator {
						t.Fatal("current checks changed immutable evidence or scan bounds")
					}
				}
			})
		}
	}
}

func TestExpiryIdentityBounds(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	_, cert := expiryIdentityCertificate(t, f)
	base := cert.Locator[expiryIdentityOldestLocator(t, cert)].Block
	for _, name := range []string{"parent-height-at-slot", "parent-height-above-slot", "first-produced-block", "non-genesis-zero-height-parent", "zero-height", "parent-after-child", "slot-above-safe", "height-above-slot", "invalid-block-hash", "invalid-parent-hash"} {
		t.Run(name, func(t *testing.T) {
			block := base
			want := false
			switch name {
			case "parent-height-at-slot":
				block.ParentSlot = block.Height - 1
				want = true
			case "parent-height-above-slot":
				block.ParentSlot = block.Height - 2
			case "first-produced-block":
				block.Slot = 2
				block.Height = 1
				block.ParentSlot = 0
				want = true
			case "non-genesis-zero-height-parent":
				block.Slot = 2
				block.Height = 1
				block.ParentSlot = 1
			case "zero-height":
				block.Height = 0
				block.ParentSlot = 0
			case "parent-after-child":
				block.ParentSlot = block.Slot + 1
			case "slot-above-safe":
				block.Slot = uint64(maxSafeInteger) + 1
			case "height-above-slot":
				block.Height = block.Slot + 1
			case "invalid-block-hash":
				block.Blockhash = "invalid"
			case "invalid-parent-hash":
				block.PreviousBlockhash = "invalid"
			}
			if err := validateExpiryBlockViews([]expiryBlock{block}); (err == nil) != want {
				t.Fatalf("single view admissible=%t, got %v", want, err)
			}
		})
	}
}
