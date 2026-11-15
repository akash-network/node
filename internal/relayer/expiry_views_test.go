package relayer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	solana "github.com/gagliardetto/solana-go"
)

// Both anchors are individually valid, but the later one's explicit parent
// hash contradicts the earlier finalized block already held by the certifier.
func TestExpiryAdvancingAnchorParentContradiction(t *testing.T) {
	for _, cached := range []bool{false, true} {
		name := "fresh"
		if cached {
			name = "cached"
		}
		t.Run(name, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			if cached {
				saved, err := f.service.store.snapshotPending(f.path)
				if err != nil {
					t.Fatal(err)
				}
				if err := runDurablePayer(t, f.service, func(p *payerSession) error {
					cert, err := p.certifyExpired(context.Background(), saved)
					if err != nil {
						return err
					}
					return f.service.store.publishExpiry(saved, cert, expiryFiles())
				}); err != nil {
					t.Fatal(err)
				}
			}
			initialSlots := f.calls["getSlot"]
			changed := false
			f.mutate = func(method string, _ []json.RawMessage, result any) any {
				if method == "getSlot" && (cached || f.calls["getSlot"]-initialSlots == 2) {
					return expiryTestSlot(1011)
				}
				if method == "getBlock" {
					block := result.(map[string]any)
					if block["blockHeight"] == uint64(1011) {
						if block["parentSlot"] != expiryTestSlot(1010) {
							t.Error("fixture did not point at the previously observed anchor")
						}
						block["previousBlockhash"] = expiryTestHash(777).String()
						changed = true
					}
				}
				return result
			}
			defer func() {
				if !changed {
					t.Error("contradictory finalized parent response was not exercised")
				}
			}()
			if err := f.retained(context.Background()); !strings.Contains(err.Error(), "conflicting expiry parent views") {
				t.Fatalf("wrong contradiction failure: %v", err)
			}
		})
	}
}

func TestExpiryAdvancingAnchorAncestorOrder(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, mode := range []string{"skips-observed-anchor", "reuses-observed-height"} {
			name := "fresh/" + mode
			if cached {
				name = "cached/" + mode
			}
			t.Run(name, func(t *testing.T) {
				f := newExpiryFixture(t, "proof-append")
				if cached {
					saved, err := f.service.store.snapshotPending(f.path)
					if err != nil {
						t.Fatal(err)
					}
					if err := runDurablePayer(t, f.service, func(p *payerSession) error {
						cert, err := p.certifyExpired(context.Background(), saved)
						if err != nil {
							return err
						}
						return f.service.store.publishExpiry(saved, cert, expiryFiles())
					}); err != nil {
						t.Fatal(err)
					}
				}
				initialSlots := f.calls["getSlot"]
				changed := false
				f.mutate = func(method string, _ []json.RawMessage, result any) any {
					if method == "getSlot" && (cached || f.calls["getSlot"]-initialSlots == 2) {
						return expiryTestSlot(1011)
					}
					if method == "getBlock" {
						block := result.(map[string]any)
						if block["blockHeight"] == uint64(1011) {
							block["parentSlot"] = expiryTestSlot(1009)
							block["previousBlockhash"] = expiryTestHash(1009).String()
							if mode == "reuses-observed-height" {
								block["parentSlot"] = expiryTestSlot(1010) + 1
								block["previousBlockhash"] = expiryTestHash(777).String()
							}
							changed = true
						}
					}
					return result
				}
				if err := f.retained(context.Background()); !strings.Contains(err.Error(), "conflicting expiry parent order") {
					t.Fatalf("wrong ancestor-order failure: %v", err)
				}
				if !changed {
					t.Fatal("advancing anchor was not exercised")
				}
			})
		}
	}
}

func TestExpiryOmittedLocatorInteriorStillChecksParent(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	checked := false
	f.mutate = func(method string, _ []json.RawMessage, result any) any {
		if method == "getBlocks" {
			out := []uint64{}
			for _, slot := range result.([]uint64) {
				if slot != expiryTestSlot(900) {
					out = append(out, slot)
				}
			}
			return out
		}
		if method == "getBlock" {
			block := result.(map[string]any)
			if block["blockHeight"] == uint64(900) {
				block["signatures"] = []string{f.doc.Signature}
				checked = true
			}
		}
		return result
	}
	f.retained(context.Background())
	if !checked {
		t.Fatal("parent walk did not inspect the omitted locator entry")
	}
}

func TestExpiryCertificateKnownParentHeight(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
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

	// Individually valid advancing anchor, explicitly naming the known parent
	// and its correct hash, but skipping a produced block height.
	cert.Final.Anchor = cert.Initial.Anchor
	cert.Final.Anchor.Slot += 4
	cert.Final.Anchor.Height += 2
	cert.Final.Anchor.ParentSlot = cert.Initial.Anchor.Slot
	cert.Final.Anchor.PreviousBlockhash = cert.Initial.Anchor.Blockhash
	cert.Final.Anchor.Blockhash = expiryTestHash(1012).String()
	if err := validateExpiryCertificate(saved, cert); err == nil || !strings.Contains(err.Error(), "conflicting expiry parent views") {
		t.Fatalf("known parent height contradiction accepted: %v", err)
	}
}

func TestExpiryAdvancingAnchorControls(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, advance := range []uint64{1, 2} {
			name := "fresh"
			if cached {
				name = "cached"
			}
			if advance == 2 {
				name += "/unobserved-parent"
			} else {
				name += "/observed-parent"
			}
			t.Run(name, func(t *testing.T) {
				f := newExpiryFixture(t, "proof-append")
				if cached {
					saved, err := f.service.store.snapshotPending(f.path)
					if err != nil {
						t.Fatal(err)
					}
					if err := runDurablePayer(t, f.service, func(p *payerSession) error {
						cert, err := p.certifyExpired(context.Background(), saved)
						if err != nil {
							return err
						}
						return f.service.store.publishExpiry(saved, cert, expiryFiles())
					}); err != nil {
						t.Fatal(err)
					}
				}
				initialSlots := f.calls["getSlot"]
				f.mutate = func(method string, _ []json.RawMessage, result any) any {
					if method == "getSlot" && (cached || f.calls["getSlot"]-initialSlots == 2) {
						return expiryTestSlot(1010 + advance)
					}
					return result
				}
				if err := f.resume(context.Background()); !isIntentNotIncluded(err) {
					t.Fatalf("coherent advancing anchor refused: %v", err)
				}
				if f.observed != 0 || f.calls["sendTransaction"] != 0 {
					t.Fatal("certification sent or observed a transaction")
				}
			})
		}
	}
}

// Enumerate every produced/skipped-slot chain in a finite domain. Compare
// independently possible pairs of full observations with the consistency gate.
func TestExpiryObservedAncestorModel(t *testing.T) {
	type observed struct {
		slot   uint64
		height uint64
		parent uint64
	}
	views := make(map[observed]expiryBlock)
	possible := make(map[[2]observed]bool)
	for mask := uint64(1); mask < 1<<7; mask++ {
		var chain []observed
		var parent uint64
		for slot := uint64(1); slot <= 7; slot++ {
			if mask&(1<<(slot-1)) == 0 {
				continue
			}
			key := observed{
				slot:   slot,
				height: uint64(len(chain) + 1),
				parent: parent,
			}
			chain = append(chain, key)
			views[key] = expiryBlock{
				Slot:              slot,
				Height:            key.height,
				ParentSlot:        parent,
				Blockhash:         expiryTestHash(slot + 1).String(),
				PreviousBlockhash: expiryTestHash(parent + 1).String(),
				Signatures:        []string{(solana.Signature{byte(slot + 1)}).String()},
			}
			parent = slot
		}
		for _, first := range chain {
			for _, second := range chain {
				possible[[2]observed{first, second}] = true
			}
		}
	}

	checked, mismatches := 0, 0
	for first, a := range views {
		for second, b := range views {
			checked++
			want := possible[[2]observed{first, second}]
			err := validateExpiryBlockViews([]expiryBlock{a, b})
			if (err == nil) != want {
				mismatches++
				if mismatches == 1 {
					t.Logf("first disagreement: first=%+v second=%+v possible=%t error=%v", first, second, want, err)
				}
			}
		}
	}
	if mismatches != 0 {
		t.Fatalf("%d of %d observation pairs disagree with enumerated chains", mismatches, checked)
	}
	t.Logf("%d observation pairs match %d coherent pairs from 127 produced-slot chains", checked, len(possible))
}
