package relayer

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDurableJournalFailureJSONAndOmissions(t *testing.T) {
	for _, failure := range []string{`null`, `{"InstructionError":[0,"InvalidArgument"]}`, `[]`, `"rpc-specific-error"`, `17`, `false`} {
		var doc transactionDoc
		data := []byte(`{"stage":"assembly","signature":"","error":` + failure + `,"future":null}`)
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		var restored transactionDoc
		if err := json.Unmarshal(encoded, &restored); err != nil || string(restored.Error) != failure || string(restored.Extra["future"]) != "null" {
			t.Fatalf("failure JSON changed: %s %v", encoded, err)
		}
		if bytes.Contains(encoded, []byte(`"payerLamports"`)) {
			t.Fatal("optional payer change should be omitted")
		}
	}
	for _, data := range []string{
		`{"stage":"assembly","signature":""}`,
		`{"stage":"assembly","error":null}`,
		`{"stage":"assembly","signature":null,"error":null}`,
	} {
		var doc transactionDoc
		if err := json.Unmarshal([]byte(data), &doc); err == nil {
			t.Fatalf("incomplete terminal record accepted: %s", data)
		}
	}
	for _, signatures := range [][]string{nil, {}} {
		entry := spendEntry{Signatures: signatures}
		encoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(encoded, []byte(`"signatures"`)) != (signatures != nil) || bytes.Contains(encoded, []byte(`"retry"`)) {
			t.Fatalf("legacy omission changed: %s", encoded)
		}
	}
}
