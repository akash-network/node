package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func expiryStateJournal(t *testing.T, path, signature string, minimum int64) {
	t.Helper()
	record, err := json.Marshal(map[string]any{
		"event":     "transaction",
		"signature": signature,
		"logs":      []string{strings.Repeat("unrelated history ", 2048)},
	})
	require.NoError(t, err)
	record = append(record, '\n')
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	file, err := os.Create(path)
	require.NoError(t, err)
	t.Cleanup(func() { file.Close() })
	var size int64
	for size <= minimum {
		n, err := file.Write(record)
		require.NoError(t, err)
		size += int64(n)
	}
	require.NoError(t, file.Close())
}

func TestExpiryStateUnboundedJournalHistory(t *testing.T) {
	for _, stage := range []string{"proof-append", "signature"} {
		t.Run(stage, func(t *testing.T) {
			f := newExpiryFixture(t, stage)
			s := f.service.store
			path := filepath.Join(s.headerDirectory, "transactions.jsonl")
			expiryStateJournal(t, path, solana.Signature{1}.String(), 64<<20)
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Greater(t, info.Size(), int64(64<<20))
			require.True(t, isIntentNotIncluded(f.resume(context.Background())))
			require.Zero(t, f.observed)
			require.Zero(t, f.calls["sendTransaction"])
		})
	}
}

func TestExpiryStateLateJournalObservation(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	s := f.service.store
	expiryStateJournal(t, filepath.Join(s.headerDirectory, "transactions.jsonl"), solana.Signature{1}.String(), 64<<20)
	require.NoError(t, s.appendHeaderEvent(map[string]any{
		"event":         "authenticated-concurrently",
		"localAssembly": f.doc.Signature,
	}))
	err := f.retained(context.Background())
	require.ErrorContains(t, err, "header journal observation contradicts")
	require.Zero(t, f.calls["getBlock"])
}

func expiryStateSessions(t *testing.T, f *expiryFixture, count int) string {
	t.Helper()
	s := f.service.store
	require.NoError(t, runDurablePayer(t, f.service, func(*payerSession) error { return nil }))
	id := strings.TrimSuffix(filepath.Base(f.path), ".pending.json")
	doc, err := s.loadSession(id)
	require.NoError(t, err)
	for i := 0; i < count; i++ {
		doc.Session = fmt.Sprintf("%064x", i+1)
		data, err := json.Marshal(doc)
		require.NoError(t, err)
		path, err := s.sessionPath(doc.Session, ".json")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600))
	}
	// Read in filesystem enumeration order so the terminal witness is in a late
	// batch on filesystems whose directory iteration is not lexicographical.
	directory, err := os.Open(s.headerDirectory)
	require.NoError(t, err)
	entries, err := directory.ReadDir(-1)
	require.NoError(t, err)
	require.NoError(t, directory.Close())
	for i := len(entries) - 1; i >= 0; i-- {
		id := strings.TrimSuffix(entries[i].Name(), ".json")
		if validHex(id, 32, true) {
			return id
		}
	}
	t.Fatal("no session in directory")
	return ""
}

func TestExpiryStateUnboundedSessionHistory(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprint(terminal), func(t *testing.T) {
			f := newExpiryFixture(t, "signature")
			s := f.service.store
			last := expiryStateSessions(t, f, 4100)
			if terminal {
				doc, err := s.loadSession(last)
				require.NoError(t, err)
				doc.LookupCloseTransaction = &f.doc.Signature
				require.NoError(t, s.saveSession(last, doc))
				err = f.retained(context.Background())
				require.ErrorContains(t, err, "session observation contradicts")
				require.Zero(t, f.calls["getBlock"])
				return
			}
			require.True(t, isIntentNotIncluded(f.resume(context.Background())))
		})
	}
}

func TestExpiryStateWholeLedgerHasNoExpiryByteCap(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	s := f.service.store
	require.NoError(t, runDurablePayer(t, f.service, func(*payerSession) error { return nil }))
	ledger := (&spendLedger{}).document(s.identity.Genesis, s.identity.ProgramID)
	data, err := json.Marshal(ledger)
	require.NoError(t, err)
	file, err := os.Create(s.ledgerPath)
	require.NoError(t, err)
	t.Cleanup(func() { file.Close() })
	_, err = file.Write(data)
	require.NoError(t, err)
	padding := bytes.Repeat([]byte{' '}, 1<<20)
	for i := 0; i < 65; i++ {
		_, err = file.Write(padding)
		require.NoError(t, err)
	}
	require.NoError(t, file.Close())
	saved, err := s.snapshotPending(f.path)
	require.NoError(t, err)
	require.NoError(t, s.expiryContradictions(context.Background(), saved))
}

func TestExpiryStateStreamRefusesChanges(t *testing.T) {
	for _, mode := range []string{"append", "rewrite", "replace", "symlink", "remove", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			root := durableTestDirectory(t)
			path := filepath.Join(root, "journal")
			require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := expiryStream(ctx, root, path, func(reader io.Reader) error {
				_, err := io.Copy(io.Discard, reader)
				require.NoError(t, err)
				switch mode {
				case "append":
					require.NoError(t, os.WriteFile(path, []byte("original appended"), 0o600))
				case "rewrite":
					require.NoError(t, os.WriteFile(path, []byte("replaced"), 0o600))
				case "replace":
					temporary := filepath.Join(root, "replacement")
					require.NoError(t, os.WriteFile(temporary, []byte("original"), 0o600))
					require.NoError(t, os.Rename(temporary, path))
				case "symlink":
					real := filepath.Join(root, "real")
					require.NoError(t, os.Rename(path, real))
					require.NoError(t, os.Symlink(real, path))
				case "remove":
					require.NoError(t, os.Remove(path))
				case "cancel":
					cancel()
				}
				return nil
			})
			require.Error(t, err)
			if mode == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestExpiryStateMalformedJournalRetains(t *testing.T) {
	for _, line := range []string{`{"event":"transaction","signature":`, "\n", strings.Repeat(" ", 2<<20)} {
		t.Run(fmt.Sprint(len(line)), func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			s := f.service.store
			require.NoError(t, os.MkdirAll(s.headerDirectory, 0o700))
			path := filepath.Join(s.headerDirectory, "transactions.jsonl")
			require.NoError(t, os.WriteFile(path, []byte(line), 0o600))
			err := f.retained(context.Background())
			require.ErrorContains(t, err, "expiry header journal line 1")
			require.ErrorContains(t, err, "repair from verified history")
			require.Zero(t, f.calls["getBlock"])
			if len(line) < 2<<20 {
				require.NoError(t, s.appendHeaderEvent(map[string]any{
					"event":     "submitted",
					"signature": solana.Signature{2}.String(),
				}))
				saved, err := s.snapshotPending(f.path)
				require.NoError(t, err)
				require.Error(t, s.expiryContradictions(context.Background(), saved))
			}
		})
	}
}

func TestExpiryStateStreamRejectsSpecialFilesAndCancellation(t *testing.T) {
	root := durableTestDirectory(t)
	path := filepath.Join(root, "target")
	require.NoError(t, os.WriteFile(path, []byte("valid"), 0o600))
	link := filepath.Join(root, "symlink")
	require.NoError(t, os.Symlink(path, link))
	for _, invalid := range []string{link, root} {
		require.Error(t, expiryStream(context.Background(), root, invalid, func(io.Reader) error {
			t.Fatal("invalid file was consumed")
			return nil
		}))
	}
	ctx, cancel := context.WithCancel(context.Background())
	err := expiryStream(ctx, root, path, func(reader io.Reader) error {
		cancel()
		_, err := io.ReadAll(reader)
		return err
	})
	require.True(t, errors.Is(err, context.Canceled))
}

func expiryStatePublished(t *testing.T, f *expiryFixture) (pendingSnapshot, *nonInclusionCertificate, []byte) {
	t.Helper()
	saved, err := f.service.store.snapshotPending(f.path)
	require.NoError(t, err)
	var cert *nonInclusionCertificate
	require.NoError(t, runDurablePayer(t, f.service, func(p *payerSession) error {
		var err error
		cert, err = p.certifyExpired(context.Background(), saved)
		if err != nil {
			return err
		}
		return f.service.store.publishExpiry(saved, cert, expiryFiles())
	}))
	data, err := os.ReadFile(f.service.store.expiryCapsulePath(saved))
	require.NoError(t, err)
	return saved, cert, data
}

func expiryStateRestart(t *testing.T, f *expiryFixture) {
	t.Helper()
	prior := f.service.store
	identity := prior.identity
	store, err := newStateStore(prior.expiryRoot(), identity.ChainID, identity.Genesis, identity.ProgramID, identity.Payer, nil)
	require.NoError(t, err)
	f.service.store = store
	f.service.expiryCooldowns = nil
}

func expiryStateEndpoint(t *testing.T, f *expiryFixture) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	f.service.solana = rpc.New(server.URL)
	f.service.cfg.solanaRPC = server.URL
	endpoint, err := expiryEndpoint(server.URL)
	require.NoError(t, err)
	return endpoint
}

func TestExpiryStateAssociationDriftRestart(t *testing.T) {
	for _, stage := range []string{"proof-append", "lookup-create"} {
		t.Run(stage, func(t *testing.T) {
			f := newExpiryFixture(t, stage)
			saved, _, original := expiryStatePublished(t, f)
			s := f.service.store
			if stage == "lookup-create" {
				f.service.reader = bridge.NewReader(f.service.solana, f.service.cfg.program, f.service.cfg.chainID, nil)
				require.NoError(t, runDurablePayer(t, f.service, func(p *payerSession) error {
					return p.reclaimLookups(context.Background())
				}))
			} else {
				proof, err := os.ReadFile(f.path + ".proof")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(f.path+".proof", append(proof, '\n'), 0o600))
			}
			current, err := s.snapshotPending(f.path)
			require.NoError(t, err)
			require.Equal(t, saved.Digest, current.Digest)
			require.Equal(t, saved.Original, current.Original)
			require.NotEqual(t, saved.Associated, current.Associated)
			expiryStateRestart(t, f)
			blocks, locators := f.calls["getBlock"], f.calls["getBlocks"]
			require.True(t, isIntentNotIncluded(f.resume(context.Background())))
			require.Equal(t, blocks+1, f.calls["getBlock"], "reuse must check the current anchor")
			require.Equal(t, locators, f.calls["getBlocks"])
			after, err := os.ReadFile(s.expiryCapsulePath(saved))
			require.NoError(t, err)
			require.Equal(t, original, after)
		})
	}
}

func TestExpiryStateDriftStillRequiresFreshEvidence(t *testing.T) {
	for _, mode := range []string{"anchor-conflict", "valid-blockhash", "ledger", "missing-association", "changed-before-unlink"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			saved, _, original := expiryStatePublished(t, f)
			proof, err := os.ReadFile(f.path + ".proof")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(f.path+".proof", append(proof, '\n'), 0o600))
			s := f.service.store
			switch mode {
			case "ledger":
				_, err := s.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
					return true, ledger.charge(f.id, 5000, f.doc.Signature, 0)
				})
				require.NoError(t, err)
			case "missing-association":
				require.NoError(t, os.Remove(f.path+".proof"))
			}
			f.mutate = func(method string, _ []json.RawMessage, result any) any {
				if mode == "anchor-conflict" && method == "getBlock" {
					result.(map[string]any)["signatures"] = []string{solana.Signature{7}.String()}
				}
				if mode == "valid-blockhash" && method == "isBlockhashValid" {
					result.(map[string]any)["value"] = true
				}
				return result
			}
			expiryStateRestart(t, f)
			if mode == "changed-before-unlink" {
				current, err := s.snapshotPending(f.path)
				require.NoError(t, err)
				err = runDurablePayer(t, f.service, func(p *payerSession) error {
					cert, err := p.certifyExpired(context.Background(), current)
					if err != nil {
						return err
					}
					ops := expiryFiles()
					ops.sync = func(file *os.File) error {
						if file.Name() == s.expiryCapsulePath(saved) {
							if err := os.WriteFile(f.path+".proof", proof, 0o600); err != nil {
								return err
							}
						}
						return file.Sync()
					}
					return p.retireExpiredWith(context.Background(), current, cert, ops)
				})
				require.ErrorContains(t, err, "associated identity changed")
				pending, err := os.ReadFile(f.path)
				require.NoError(t, err)
				require.Equal(t, saved.Original, pending)
			} else {
				f.retained(context.Background())
			}
			after, err := os.ReadFile(s.expiryCapsulePath(saved))
			require.NoError(t, err)
			require.Equal(t, original, after)
		})
	}
}

func TestExpiryStateProviderGenerationRestart(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	saved, primary, original := expiryStatePublished(t, f)
	endpoint := expiryStateEndpoint(t, f)
	require.NotEqual(t, primary.Endpoint, endpoint)
	blocks := f.calls["getBlock"]
	injected := errors.New("interrupt after generation publication")
	err := runDurablePayer(t, f.service, func(p *payerSession) error {
		cert, err := p.certifyExpired(context.Background(), saved)
		if err != nil {
			return err
		}
		require.Equal(t, endpoint, cert.Endpoint)
		require.GreaterOrEqual(t, f.calls["getBlock"]-blocks, expiryWindowBlocks)
		ops := expiryFiles()
		ops.unlink = func(string) error { return injected }
		return p.retireExpiredWith(context.Background(), saved, cert, ops)
	})
	require.ErrorIs(t, err, injected)
	s := f.service.store
	generationPath := s.expiryGenerationPath(saved, endpoint)
	generation, err := os.ReadFile(generationPath)
	require.NoError(t, err)
	pending, err := os.ReadFile(f.path)
	require.NoError(t, err)
	require.Equal(t, saved.Original, pending)
	expiryStateRestart(t, f)
	blocks, locators := f.calls["getBlock"], f.calls["getBlocks"]
	require.True(t, isIntentNotIncluded(f.resume(context.Background())))
	require.Equal(t, blocks+1, f.calls["getBlock"])
	require.Equal(t, locators, f.calls["getBlocks"])
	after, err := os.ReadFile(generationPath)
	require.NoError(t, err)
	require.Equal(t, generation, after)
	after, err = os.ReadFile(s.expiryCapsulePath(saved))
	require.NoError(t, err)
	require.Equal(t, original, after)
	loaded, err := s.loadExpiryCertificateFor(saved, primary.Endpoint)
	require.NoError(t, err)
	require.Equal(t, primary, loaded)
}

func TestExpiryStateProviderDriftRefusesContradictions(t *testing.T) {
	for _, mode := range []string{"corrupt-primary", "different-history", "corrupt-generation", "terminal-ledger"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			saved, _, _ := expiryStatePublished(t, f)
			s := f.service.store
			endpoint := expiryStateEndpoint(t, f)
			switch mode {
			case "corrupt-primary":
				require.NoError(t, os.WriteFile(s.expiryCapsulePath(saved), []byte("{torn"), 0o600))
			case "different-history":
				f.mutate = func(method string, _ []json.RawMessage, result any) any {
					if method == "getBlock" {
						result.(map[string]any)["signatures"] = []string{solana.Signature{8}.String()}
					}
					return result
				}
			case "corrupt-generation":
				require.NoError(t, os.WriteFile(s.expiryGenerationPath(saved, endpoint), []byte("{torn"), 0o600))
			case "terminal-ledger":
				_, err := s.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
					return true, ledger.charge(f.id, 5000, f.doc.Signature, 0)
				})
				require.NoError(t, err)
			}
			blocks := f.calls["getBlock"]
			err := f.retained(context.Background())
			if mode == "different-history" {
				require.ErrorContains(t, err, "generation contradicts original evidence")
			} else {
				require.Equal(t, blocks, f.calls["getBlock"])
			}
		})
	}
}

func expiryStateReceipt(f *expiryFixture, confirmed bool) {
	balances := make([]int64, f.doc.Accounts)
	balances[0] = 10000000
	after := append([]int64{}, balances...)
	after[0] -= 5000
	f.mutate = func(method string, _ []json.RawMessage, result any) any {
		if confirmed && method == "getSignatureStatuses" {
			result.(map[string]any)["value"] = []any{
				map[string]any{
					"slot":               2900,
					"confirmations":      1,
					"confirmationStatus": "confirmed",
					"err":                nil,
				},
			}
		}
		if method != "getTransaction" {
			return result
		}
		return map[string]any{
			"slot":        2900,
			"version":     0,
			"transaction": []string{f.doc.Raw, "base64"},
			"meta": map[string]any{
				"err":                  nil,
				"fee":                  5000,
				"computeUnitsConsumed": 123,
				"preBalances":          balances,
				"postBalances":         after,
				"logMessages":          []string{"observed receipt"},
				"loadedAddresses": map[string]any{
					"writable": []string{},
					"readonly": []string{},
				},
			},
		}
	}
	if confirmed {
		// The shared expiry fixture normally requires finalized receipt requests.
		// These cases intentionally exercise baseline confirmed-only settlement.
		f.reply = func(w http.ResponseWriter, id json.RawMessage, method string) bool {
			if method != "getTransaction" {
				return false
			}
			if err := json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  f.mutate(method, nil, nil),
			}); err != nil {
				f.t.Error(err)
			}
			return true
		}
	}

}

func TestExpiryStateReceiptSupersedesAllGenerations(t *testing.T) {
	for _, stage := range []string{"proof-append", "lookup-create", "signature"} {
		for _, association := range []string{"unchanged", "changed", "missing"} {
			t.Run(stage+"/"+association, func(t *testing.T) {
				f := newExpiryFixture(t, stage)
				saved, primary, original := expiryStatePublished(t, f)
				endpoint := expiryStateEndpoint(t, f)
				expiryStatePublished(t, f)
				s := f.service.store
				generationPath := s.expiryGenerationPath(saved, endpoint)
				generation, err := os.ReadFile(generationPath)
				require.NoError(t, err)
				path := filepath.Join(s.expiryRoot(), saved.Associated[0].Path)
				switch association {
				case "changed":
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600))
				case "missing":
					require.NoError(t, os.Remove(path))
				}
				var alerts bytes.Buffer
				f.service.deps.streams.Err = &alerts
				expiryStateReceipt(f, stage == "signature")
				observed := 0
				require.NoError(t, runDurablePayer(t, f.service, func(p *payerSession) error {
					result, err := p.resume(context.Background(), f.path, func(result txResult) error {
						observed++
						require.Equal(t, uint64(5000), result.fee)
						require.Equal(t, int64(5000), *result.payerLamports)
						marker, err := s.readExpirySupersession(saved)
						require.NoError(t, err)
						require.NotNil(t, marker, "marker must precede observer")
						return nil
					})
					require.NotNil(t, result)
					return err
				}))
				require.Equal(t, 1, observed)
				require.Contains(t, alerts.String(), "expiry-certificate-superseded")
				_, err = os.Stat(f.path)
				require.ErrorIs(t, err, os.ErrNotExist)
				for _, provider := range []string{primary.Endpoint, endpoint} {
					loaded, err := s.loadExpiryCertificateFor(saved, provider)
					require.Nil(t, loaded)
					require.ErrorContains(t, err, "superseded")
				}
				_, err = s.loadExpiryCertificate(saved)
				require.ErrorContains(t, err, "superseded")
				after, err := os.ReadFile(s.expiryCapsulePath(saved))
				require.NoError(t, err)
				require.Equal(t, original, after)
				after, err = os.ReadFile(generationPath)
				require.NoError(t, err)
				require.Equal(t, generation, after)
				// Reintroducing exact pending bytes cannot revive any old capsule.
				require.NoError(t, os.WriteFile(f.path, saved.Original, 0o600))
				require.ErrorContains(t, runDurablePayer(t, f.service, func(p *payerSession) error {
					return p.retireExpired(context.Background(), saved, primary)
				}), "superseded")
			})
		}
	}
}

func TestExpiryStateReceiptRestartDeduplicatesCharge(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	saved, _, original := expiryStatePublished(t, f)
	expiryStateReceipt(f, false)
	observed := 0
	injected := errors.New("observer interrupted after durable charge")
	resume := func(fail bool) error {
		return runDurablePayer(t, f.service, func(p *payerSession) error {
			_, err := p.resume(context.Background(), f.path, func(result txResult) error {
				observed++
				if err := p.chargeResult(context.Background(), f.id, result); err != nil {
					return err
				}
				if fail {
					return injected
				}
				return nil
			})
			return err
		})
	}
	require.ErrorIs(t, resume(true), injected)
	marker, err := os.ReadFile(f.service.store.expirySupersessionPath(saved))
	require.NoError(t, err)
	expiryStateRestart(t, f)
	require.NoError(t, resume(false))
	require.Equal(t, 2, observed, "generic observer is retried after crash")
	ledger, err := f.service.store.readLedger()
	require.NoError(t, err)
	require.Equal(t, int64(5000), ledger.spent(f.id))
	require.Equal(t, []string{f.doc.Signature}, ledger.entries[f.id].Signatures)
	after, err := os.ReadFile(f.service.store.expirySupersessionPath(saved))
	require.NoError(t, err)
	require.Equal(t, marker, after)
	after, err = os.ReadFile(f.service.store.expiryCapsulePath(saved))
	require.NoError(t, err)
	require.Equal(t, original, after)
}

func TestExpiryStateReceiptPublicationFailureRestart(t *testing.T) {
	for _, failure := range []string{"write", "short-write", "file-sync", "rename", "directory-sync"} {
		t.Run(failure, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			saved, _, _ := expiryStatePublished(t, f)
			expiryStateReceipt(f, false)
			injected := errors.New("injected supersession publication failure")
			err := runDurablePayer(t, f.service, func(p *payerSession) error {
				result, err := p.settle(context.Background(), &f.doc, nil)
				if err != nil {
					return err
				}
				ops := expiryFiles()
				published := false
				ops.write = func(file *os.File, data []byte) (int, error) {
					if failure == "write" {
						return 0, injected
					}
					if failure == "short-write" {
						return file.Write(data[:len(data)/2])
					}
					return file.Write(data)
				}
				ops.rename = func(from, to string) error {
					if failure == "rename" {
						return injected
					}
					if err := os.Rename(from, to); err != nil {
						return err
					}
					published = true
					return nil
				}
				ops.sync = func(file *os.File) error {
					info, err := file.Stat()
					if err != nil {
						return err
					}
					if (failure == "file-sync" && info.Mode().IsRegular()) || (failure == "directory-sync" && published && info.IsDir()) {
						return injected
					}
					return file.Sync()
				}
				return p.reconcileExpiryReceiptWith(f.path, &f.doc, result, ops)
			})
			if failure == "short-write" {
				require.ErrorIs(t, err, io.ErrShortWrite)
			} else {
				require.ErrorIs(t, err, injected)
			}
			pending, err := os.ReadFile(f.path)
			require.NoError(t, err)
			require.Equal(t, saved.Original, pending)
			expiryStateRestart(t, f)
			observed := 0
			require.NoError(t, runDurablePayer(t, f.service, func(p *payerSession) error {
				_, err := p.resume(context.Background(), f.path, func(txResult) error {
					observed++
					return nil
				})
				return err
			}))
			require.Equal(t, 1, observed)
		})
	}
}

func TestExpiryStateReceiptRefusesConflictingEvidence(t *testing.T) {
	for _, mode := range []string{"marker-write-blocked", "malformed-marker", "different-receipt", "contradictory-capsule"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpiryFixture(t, "proof-append")
			saved, _, _ := expiryStatePublished(t, f)
			s := f.service.store
			expiryStateReceipt(f, false)
			path := s.expirySupersessionPath(saved)
			switch mode {
			case "marker-write-blocked":
				require.NoError(t, os.Mkdir(path, 0o700))
			case "malformed-marker":
				require.NoError(t, os.WriteFile(path, []byte("{torn"), 0o600))
			case "different-receipt":
				require.NoError(t, runDurablePayer(t, f.service, func(p *payerSession) error {
					result, err := p.settle(context.Background(), &f.doc, nil)
					if err != nil {
						return err
					}
					result.fee++
					return p.reconcileExpiryReceipt(f.path, &f.doc, result)
				}))
			case "contradictory-capsule":
				capsule, err := s.readExpiryCapsule(saved, s.expiryCapsulePath(saved))
				require.NoError(t, err)
				capsule.Certificate.Blocks[20].Signatures = []string{f.doc.Signature}
				certificate, err := json.Marshal(capsule.Certificate)
				require.NoError(t, err)
				capsule.CertificateHash = digestHex(certificate)
				data, err := json.Marshal(capsule)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(s.expiryCapsulePath(saved), append(data, '\n'), 0o600))
			}
			f.retained(context.Background())
		})
	}
}

func TestExpiryStateNoCapsuleReceiptKeepsBaseline(t *testing.T) {
	for _, stage := range []string{"proof-append", "signature", "chunk"} {
		t.Run(stage, func(t *testing.T) {
			f := newExpiryFixture(t, stage)
			saved, err := f.service.store.snapshotPending(f.path)
			require.NoError(t, err)
			require.NoError(t, os.Remove(filepath.Join(f.service.store.expiryRoot(), saved.Associated[0].Path)))
			expiryStateReceipt(f, stage != "proof-append")
			observed := 0
			require.NoError(t, runDurablePayer(t, f.service, func(p *payerSession) error {
				result, err := p.resume(context.Background(), f.path, func(result txResult) error {
					observed++
					require.Equal(t, uint64(5000), result.fee)
					return nil
				})
				require.NotNil(t, result)
				return err
			}))
			require.Equal(t, 1, observed)
			_, err = os.Stat(f.service.store.expirySupersessionPath(saved))
			require.ErrorIs(t, err, os.ErrNotExist)
			require.Zero(t, f.calls["getBlock"])
		})
	}
}

type expiryStateWalkContext struct {
	context.Context
	checks int
	action func(int)
}

func (c *expiryStateWalkContext) Err() error {
	c.checks++
	c.action(c.checks)
	return c.Context.Err()
}

func TestExpiryStateSessionWalkChangesAndCancellation(t *testing.T) {
	for _, mode := range []string{"replace-directory", "append-entry", "symlink-directory", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpiryFixture(t, "signature")
			expiryStateSessions(t, f, 300)
			s := f.service.store
			saved, err := s.snapshotPending(f.path)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			walk := &expiryStateWalkContext{
				Context: ctx,
				action: func(check int) {
					if check != 200 {
						return
					}
					switch mode {
					case "replace-directory", "symlink-directory":
						prior := s.headerDirectory + "-prior"
						require.NoError(t, os.Rename(s.headerDirectory, prior))
						if mode == "replace-directory" {
							require.NoError(t, os.Mkdir(s.headerDirectory, 0o700))
						} else {
							require.NoError(t, os.Symlink(prior, s.headerDirectory))
						}
					case "append-entry":
						require.NoError(t, os.WriteFile(filepath.Join(s.headerDirectory, "new-entry"), []byte("changed"), 0o600))
					case "cancel":
						cancel()
					}
				},
			}
			err = s.expirySessionContradictions(walk, saved)
			require.Error(t, err)
			require.GreaterOrEqual(t, walk.checks, 200)
			if mode == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestExpiryStateSymlinkRootStillRefusesAbsence(t *testing.T) {
	f := newExpiryFixture(t, "proof-append")
	s := f.service.store
	root := s.expiryRoot()
	real := root + "-real"
	require.NoError(t, os.Rename(root, real))
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(real)) })
	require.NoError(t, os.Symlink(real, root))
	require.ErrorContains(t, f.retained(context.Background()), "expiry state ancestor is not a regular directory")
	require.Zero(t, f.calls["getBlock"])
	// Ordinary positive receipt recovery without a capsule keeps its baseline
	// behavior even though this root is ineligible for absence certification.
	expiryStateReceipt(f, false)
	observed := 0
	require.NoError(t, runDurablePayer(t, f.service, func(p *payerSession) error {
		_, err := p.resume(context.Background(), f.path, func(txResult) error {
			observed++
			return nil
		})
		return err
	}))
	require.Equal(t, 1, observed)
}
