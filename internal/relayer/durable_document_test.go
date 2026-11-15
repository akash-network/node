package relayer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func durableDocumentFixture(t *testing.T, kind string) []byte {
	t.Helper()
	name := "ts-" + kind + ".json"
	if kind == "proof" {
		name = "ts-proof-transfer.json"
	}
	data := durableFixture(t, name)
	if kind != "ledger" {
		return data
	}
	// The historical fixture includes extra identity properties. The baseline
	// ledger writer receives only genesis and programId (relay.ts:734).
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "chainId")
	delete(fields, "payer")
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func reloadDurableDocument(t *testing.T, store *stateStore, kind string, data []byte) ([]byte, error) {
	t.Helper()
	id, err := native.ParseID(durableIdentity(t).ID)
	if err != nil {
		t.Fatal(err)
	}
	switch kind {
	case "ledger":
		ledger, err := parseSpendLedger(data, store.identity.Genesis, store.identity.ProgramID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(ledger.document(store.identity.Genesis, store.identity.ProgramID))
	case "proof":
		job, err := parseProofJob(data, store.identity, id)
		if err != nil {
			return nil, err
		}
		return json.Marshal(job)
	case "intent":
		path := filepath.Join(store.jobDirectory, "boundary-intent.json")
		writeFixture(t, path, data)
		doc, err := store.loadIntent(path)
		if err != nil {
			return nil, err
		}
		raw, _, err := doc.transaction()
		if err != nil {
			return nil, err
		}
		var original intent
		if err := json.Unmarshal(durableFixture(t, "ts-intent.json"), &original); err != nil {
			t.Fatal(err)
		}
		want, _, err := original.transaction()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, want) || doc.Raw != original.Raw || doc.Signature != original.Signature {
			t.Fatal("signed intent bytes changed")
		}
		return json.Marshal(doc)
	default:
		t.Fatalf("unknown document %s", kind)
		return nil, nil
	}
}

func sameDurableJSON(t *testing.T, got, want []byte) {
	t.Helper()
	var left, right any
	if err := json.Unmarshal(got, &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &right); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("document changed: got %s, want %s", got, want)
	}
}

func TestDurableDocumentNumberSpellingsAndBounds(t *testing.T) {
	store := durableStore(t)
	fields := []struct {
		kind  string
		name  string
		value string
	}{
		{"ledger", "version", "2"},
		{"ledger", "spentLamports", "-7000"},
		{"ledger", "attempts", "1"},
		{"ledger", "bufferFundedLamports", "0"},
		{"ledger", "bufferReclaimedLamports", "9000"},
		{"ledger", "failures", "1"},
		{"ledger", "retryAt", "1700000006000"},
		{"proof", "version", "1"},
		{"proof", "proofHeight", "20"},
		{"proof", "uploaded", "0"},
		{"proof", "allocated", "0"},
		{"proof", "lamports", "0"},
		{"intent", "lastValidBlockHeight", "1000"},
		{"intent", "accounts", "3"},
	}
	for _, field := range fields {
		t.Run(field.kind+"/"+field.name, func(t *testing.T) {
			data := durableDocumentFixture(t, field.kind)
			original := fmt.Sprintf(`%q:%s`, field.name, field.value)
			if !bytes.Contains(data, []byte(original)) {
				t.Fatal("missing fixture field")
			}
			for _, spelling := range []string{field.value + ".0", field.value + "e0"} {
				t.Run(spelling, func(t *testing.T) {
					changed := bytes.Replace(data, []byte(original), []byte(fmt.Sprintf(`%q:%s`, field.name, spelling)), 1)
					saved, err := reloadDurableDocument(t, store, field.kind, changed)
					if err != nil {
						t.Fatal(err)
					}
					sameDurableJSON(t, saved, data)
				})
			}
			for _, spelling := range []string{"1.5", "9007199254740992", "-9007199254740992", "1e309", "null", `"1"`, "true", "[]", "{}"} {
				t.Run("reject/"+spelling, func(t *testing.T) {
					changed := bytes.Replace(data, []byte(original), []byte(fmt.Sprintf(`%q:%s`, field.name, spelling)), 1)
					if _, err := reloadDurableDocument(t, store, field.kind, changed); err == nil {
						t.Fatalf("invalid number accepted: %s", spelling)
					}
				})
			}
		})
	}
}

func TestDurableDocumentJavaScriptRounding(t *testing.T) {
	store := durableStore(t)
	data := durableDocumentFixture(t, "ledger")
	for _, test := range []struct {
		spelling string
		want     int64
	}{
		{"9007199254740990.5", 9007199254740990},
		{"9007199254740991.4", maxSafeInteger},
		{"-9007199254740991.4", -maxSafeInteger},
		{"1.00000000000000001", 1},
		{"1e-400", 0},
		{"-0.0", 0},
	} {
		t.Run(test.spelling, func(t *testing.T) {
			changed := bytes.Replace(data, []byte(`"spentLamports":-7000`), []byte(`"spentLamports":`+test.spelling), 1)
			saved, err := reloadDurableDocument(t, store, "ledger", changed)
			if err != nil {
				t.Fatal(err)
			}
			want := bytes.Replace(data, []byte(`"spentLamports":-7000`), []byte(fmt.Sprintf(`"spentLamports":%d`, test.want)), 1)
			sameDurableJSON(t, saved, want)
		})
	}
	for _, spelling := range []string{"9007199254740991.5", "-9007199254740991.5", "1.000000000000001", "5e-324"} {
		changed := bytes.Replace(data, []byte(`"spentLamports":-7000`), []byte(`"spentLamports":`+spelling), 1)
		if _, err := reloadDurableDocument(t, store, "ledger", changed); err == nil {
			t.Fatalf("unsafe or fractional binary64 accepted: %s", spelling)
		}
	}
}

func TestDurableDocumentExactKeysAndDuplicateValues(t *testing.T) {
	store := durableStore(t)
	for _, kind := range []string{"ledger", "proof", "intent"} {
		t.Run(kind, func(t *testing.T) {
			data := durableDocumentFixture(t, kind)
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			for name, value := range fields {
				t.Run(name, func(t *testing.T) {
					property := fmt.Sprintf(`%q:%s`, name, value)
					for _, invalid := range []string{`null`, `{"discarded":true}`, `"discarded"`} {
						changed := bytes.Replace(data, []byte(property), []byte(fmt.Sprintf(`%q:%s,`, name, invalid)+property), 1)
						saved, err := reloadDurableDocument(t, store, kind, changed)
						if err != nil {
							t.Fatal(err)
						}
						sameDurableJSON(t, saved, data)
					}
					alias := strings.ToUpper(name[:1]) + name[1:]
					changed := bytes.Replace(data, []byte(property), []byte(fmt.Sprintf(`%s,%q:null`, property, alias)), 1)
					saved, err := reloadDurableDocument(t, store, kind, changed)
					if kind == "proof" {
						if err == nil {
							t.Fatal("strict proof schema accepted case alias")
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						sameDurableJSON(t, saved, data)
					}
					changed = bytes.Replace(data, []byte(property), []byte(fmt.Sprintf(`%q:%s`, alias, value)), 1)
					if _, err := reloadDurableDocument(t, store, kind, changed); err == nil {
						t.Fatal("case alias substituted for required property")
					}
				})
			}
		})
	}
	data := durableDocumentFixture(t, "ledger")
	for _, property := range []string{`"spentLamports":-7000`, `"attempts":1`, `"failures":1`, `"retryAt":1700000006000`} {
		name := strings.SplitN(property, ":", 2)[0]
		changed := bytes.Replace(data, []byte(property), []byte(name+`:null,`+property), 1)
		saved, err := reloadDurableDocument(t, store, "ledger", changed)
		if err != nil {
			t.Fatal(err)
		}
		sameDurableJSON(t, saved, data)
		changed = bytes.Replace(data, []byte(property), []byte(property+`,`+name+`:null`), 1)
		if _, err := reloadDurableDocument(t, store, "ledger", changed); err == nil {
			t.Fatal("last null duplicate ignored")
		}
	}
}

func TestDurableJournalOpaqueFieldsRemainUninterpreted(t *testing.T) {
	data := []byte(`{"chainId":"c","genesis":"g","client":"p","target":20,"trusted":10,"packet":{"future":9007199254740993},"future":{"version":2.5,"bigint":"18446744073709551615","null":null}}`)
	var active activeHeaderDoc
	if err := json.Unmarshal(data, &active); err != nil {
		t.Fatal(err)
	}
	if string(active.Packet) != `{"future":9007199254740993}` || string(active.Extra["future"]) != `{"version":2.5,"bigint":"18446744073709551615","null":null}` {
		t.Fatal("opaque header JSON changed")
	}
	encoded, err := json.Marshal(active)
	if err != nil {
		t.Fatal(err)
	}
	var restored activeHeaderDoc
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored.Packet, active.Packet) || !bytes.Equal(restored.Extra["future"], active.Extra["future"]) {
		t.Fatal("opaque header JSON changed during save")
	}
}

func TestDurableLedgerNestedAliasesAndIgnoredFields(t *testing.T) {
	store := durableStore(t)
	data := durableDocumentFixture(t, "ledger")
	for _, property := range []string{
		`"spentLamports":-7000`, `"attempts":1`, `"bufferFundedLamports":0`,
		`"bufferReclaimedLamports":9000`, `"signatures":["retained-charge"]`,
		`"failures":1`, `"retryAt":1700000006000`,
	} {
		name := strings.SplitN(property, ":", 2)[0]
		alias := `"` + strings.ToUpper(name[1:2]) + name[2:]
		changed := bytes.Replace(data, []byte(property), []byte(property+`,`+alias+`:null`), 1)
		saved, err := reloadDurableDocument(t, store, "ledger", changed)
		if err != nil {
			t.Fatal(err)
		}
		sameDurableJSON(t, saved, data)
	}
	for _, kind := range []string{"ledger", "intent"} {
		data := durableDocumentFixture(t, kind)
		changed := append([]byte(`{"future":{"unsafe":1e309,"fraction":0.5,"null":null},`), data[1:]...)
		saved, err := reloadDurableDocument(t, store, kind, changed)
		if err != nil {
			t.Fatal(err)
		}
		sameDurableJSON(t, saved, data)
	}
}
