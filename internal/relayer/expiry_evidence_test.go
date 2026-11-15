package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	solana "github.com/gagliardetto/solana-go"
)

func checkExpiryEvidence(t *testing.T, f *expiryFixture, retire bool) {
	t.Helper()
	s := f.service.store
	_, err := s.updateLedger(context.Background(), func(l *spendLedger) (bool, error) {
		if err := l.attempt(f.id); err != nil {
			return false, err
		}
		return true, l.charge(f.id, 15000, "earlier-observed", 100000)
	})
	if err != nil {
		t.Fatal(err)
	}
	before := make(map[string][]byte)
	if err := filepath.WalkDir(s.expiryRoot(), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		before[path] = data
		return err
	}); err != nil {
		t.Fatal(err)
	}

	err = f.resume(context.Background())
	if retire {
		if !isIntentNotIncluded(err) {
			t.Errorf("legitimate evidence prevented retirement: %v", err)
		}
	} else if err == nil || isIntentNotIncluded(err) {
		t.Errorf("contradictory or malformed evidence permitted retirement: %v", err)
	}
	for path, original := range before {
		data, err := os.ReadFile(path)
		if retire && path == f.path {
			if !errors.Is(err, os.ErrNotExist) {
				t.Error("retired pending file remains", err)
			}
			continue
		}
		if err != nil || !bytes.Equal(data, original) {
			t.Errorf("evidence changed %s: %v", filepath.Base(path), err)
		}
	}
	if f.observed != 0 || f.calls["sendTransaction"] != 0 {
		t.Fatalf("evidence check observed=%d sends=%d", f.observed, f.calls["sendTransaction"])
	}
}

func TestExpiryJournalTerminalEvidence(t *testing.T) {
	for _, event := range []string{"transaction", "authenticated-concurrently", "submitted"} {
		for _, mode := range []string{"matching", "other", "missing", "null", "empty", "invalid", "number", "shadowed", "alias-only"} {
			t.Run(event+"/"+mode, func(t *testing.T) {
				f := newExpiryFixture(t, "assembly")
				field := "signature"
				record := map[string]any{
					"event": event,
					"stage": "assembly",
				}
				switch event {
				case "transaction":
					record["fee"] = 5000
					record["error"] = nil
				case "authenticated-concurrently":
					field = "localAssembly"
					delete(record, "stage")
					record["target"] = 30
				}
				record[field] = f.doc.Signature
				switch mode {
				case "other":
					record[field] = (solana.Signature{1}).String()
				case "missing":
					delete(record, field)
				case "null":
					record[field] = nil
				case "empty":
					record[field] = ""
				case "invalid":
					record[field] = "not-a-signature"
				case "number":
					record[field] = 7
				case "shadowed":
					record[strings.ToUpper(field)] = nil
				case "alias-only":
					delete(record, field)
					record[strings.ToUpper(field)] = f.doc.Signature
				}
				if err := f.service.store.appendHeaderEvent(record); err != nil {
					t.Fatal(err)
				}
				retire := mode == "other" || (mode == "matching" || mode == "shadowed") && event == "submitted"
				checkExpiryEvidence(t, f, retire)
			})
		}
	}
}

func TestExpiryJournalSignatureFreeEvidence(t *testing.T) {
	for _, event := range []string{"unsupported-header", "already-authenticated", "future-extension"} {
		t.Run(event, func(t *testing.T) {
			f := newExpiryFixture(t, "assembly")
			if err := f.service.store.appendHeaderEvent(map[string]any{
				"event":  event,
				"target": 30,
				"reason": "unsupported test header",
				"extra": map[string]any{
					"SIGNATURE": nil,
				},
			}); err != nil {
				t.Fatal(err)
			}
			checkExpiryEvidence(t, f, true)
		})
	}
}

func TestExpirySessionTerminalEvidence(t *testing.T) {
	for _, location := range []string{"associated", "inventory"} {
		for _, field := range []string{"assembly", "lookupCloseFailure", "lookupCloseTransaction"} {
			for _, mode := range []string{"matching", "other", "null", "missing-signature", "null-signature", "empty-signature", "invalid-signature", "number-signature", "shadowed-signature", "shadowed-terminal", "alias-only"} {
				if field == "lookupCloseTransaction" && (mode == "missing-signature" || mode == "shadowed-signature") {
					continue
				}
				t.Run(location+"/"+field+"/"+mode, func(t *testing.T) {
					f := newExpiryFixture(t, "assembly")
					s := f.service.store
					id := strings.TrimSuffix(filepath.Base(f.path), ".pending.json")
					doc, err := s.loadSession(id)
					if err != nil {
						t.Fatal(err)
					}
					if location == "inventory" {
						id = strings.Repeat("a", 64)
						doc.Session = id
					}
					data, err := json.Marshal(doc)
					if err != nil {
						t.Fatal(err)
					}
					fields, err := jsonObject(data)
					if err != nil {
						t.Fatal(err)
					}
					var signature any = f.doc.Signature
					switch mode {
					case "other":
						signature = (solana.Signature{1}).String()
					case "null-signature":
						signature = nil
					case "empty-signature":
						signature = ""
					case "invalid-signature":
						signature = "not-a-signature"
					case "number-signature":
						signature = 7
					}
					var terminal any = signature
					if field != "lookupCloseTransaction" {
						stage := "assembly"
						if field == "lookupCloseFailure" {
							stage = "lookup-close"
						}
						transaction := map[string]any{
							"stage":     stage,
							"signature": signature,
							"error":     nil,
						}
						if field == "lookupCloseFailure" {
							transaction["error"] = map[string]any{
								"InstructionError": []any{0, "InvalidArgument"},
							}
						}
						if mode == "missing-signature" {
							delete(transaction, "signature")
						}
						terminal = transaction
					}
					if mode == "null" {
						terminal = nil
					}
					raw, err := json.Marshal(terminal)
					if err != nil {
						t.Fatal(err)
					}
					if mode == "shadowed-signature" {
						raw = append(raw[:len(raw)-1], []byte(`,"SIGNATURE":null}`)...)
					}
					fields[field] = raw
					fields["future-extension"] = json.RawMessage(`{"ASSEMBLY":null}`)
					if mode == "alias-only" {
						delete(fields, field)
						fields[strings.ToUpper(field)] = raw
					}
					data, err = json.Marshal(fields)
					if err != nil {
						t.Fatal(err)
					}
					if mode == "shadowed-terminal" {
						data = append(data[:len(data)-1], []byte(`,"`+strings.ToUpper(field)+`":null}`)...)
					}
					path, err := s.sessionPath(id, ".json")
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
						t.Fatal(err)
					}
					checkExpiryEvidence(t, f, mode == "other")
				})
			}
		}
	}
}

func TestExpiryJournalOptionalEvidence(t *testing.T) {
	for _, field := range []string{"signature", "localAssembly"} {
		for _, mode := range []string{"matching", "other", "null"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				f := newExpiryFixture(t, "assembly")
				record := map[string]any{
					"event": "future-extension",
					field:   f.doc.Signature,
				}
				if mode == "other" {
					record[field] = (solana.Signature{1}).String()
				} else if mode == "null" {
					record[field] = nil
				}
				if err := f.service.store.appendHeaderEvent(record); err != nil {
					t.Fatal(err)
				}
				checkExpiryEvidence(t, f, mode == "other")
			})
		}
	}
}

func TestExpirySessionUnobservedClosure(t *testing.T) {
	f := newExpiryFixture(t, "lookup-close")
	s := f.service.store
	id := strings.TrimSuffix(filepath.Base(f.path), ".close.pending.json")
	doc, err := s.loadSession(id)
	if err != nil {
		t.Fatal(err)
	}
	closed := true
	doc.LookupClosed = &closed
	doc.LookupClosureUnobserved = &closed
	if err := s.saveSession(id, doc); err != nil {
		t.Fatal(err)
	}
	checkExpiryEvidence(t, f, true)
}

func TestExpiryEvidenceRecheckedAfterPublication(t *testing.T) {
	for _, mode := range []string{"transaction", "concurrent-assembly", "changed-association"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpiryFixture(t, "assembly")
			s := f.service.store
			saved, err := s.snapshotPending(f.path)
			if err != nil {
				t.Fatal(err)
			}
			id := strings.TrimSuffix(filepath.Base(f.path), ".pending.json")
			path, err := s.sessionPath(id, ".json")
			if err != nil {
				t.Fatal(err)
			}
			sessionBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var journalBytes []byte
			err = runDurablePayer(t, f.service, func(p *payerSession) error {
				cert, err := p.certifyExpired(context.Background(), saved)
				if err != nil {
					return err
				}
				ops := expiryFiles()
				ops.rename = func(from, to string) error {
					if err := os.Rename(from, to); err != nil {
						return err
					}
					if mode == "changed-association" {
						doc, err := s.loadSession(id)
						if err != nil {
							return err
						}
						doc.Target++
						if err := s.saveSession(id, doc); err != nil {
							return err
						}
						sessionBytes, err = os.ReadFile(path)
						return err
					}
					record := map[string]any{
						"event":     "transaction",
						"stage":     "assembly",
						"signature": f.doc.Signature,
						"error":     nil,
					}
					if mode == "concurrent-assembly" {
						record = map[string]any{
							"event":         "authenticated-concurrently",
							"target":        30,
							"localAssembly": f.doc.Signature,
						}
					}
					if err := s.appendHeaderEvent(record); err != nil {
						return err
					}
					journalBytes, err = os.ReadFile(filepath.Join(s.headerDirectory, "transactions.jsonl"))
					return err
				}
				return p.retireExpiredWith(context.Background(), saved, cert, ops)
			})
			data, readErr := os.ReadFile(f.path)
			if err == nil || readErr != nil || !bytes.Equal(data, saved.Original) || f.observed != 0 || f.calls["sendTransaction"] != 0 {
				t.Fatalf("post-publication evidence did not preserve pending: %v read=%v", err, readErr)
			}
			data, err = os.ReadFile(path)
			if err != nil || !bytes.Equal(data, sessionBytes) {
				t.Fatal("post-publication recheck changed session", err)
			}
			if journalBytes != nil {
				data, err := os.ReadFile(filepath.Join(s.headerDirectory, "transactions.jsonl"))
				if err != nil || !bytes.Equal(data, journalBytes) {
					t.Fatal("post-publication recheck changed journal", err)
				}
			}
			if _, err := os.Stat(s.expiryCapsulePath(saved)); err != nil {
				t.Fatal("durable evidence lost after recheck refusal", err)
			}
		})
	}
}
