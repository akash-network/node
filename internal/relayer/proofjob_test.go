package relayer

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func TestDurableProofLegacyBytesAndSettlementPath(t *testing.T) {
	store := durableStore(t)
	identity := durableIdentity(t)
	id, err := native.ParseID(identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := store.proofPath(id)
	for _, name := range []string{"ts-proof-transfer.json", "ts-proof-settlement.json"} {
		t.Run(name, func(t *testing.T) {
			data := durableFixture(t, name)
			writeFixture(t, path, data)
			job, err := store.loadProof(id)
			if err != nil {
				t.Fatal(err)
			}
			proof, _ := hex.DecodeString(job.Proof)
			var merkle commitment.MerkleProof
			if err := merkle.Unmarshal(proof); err != nil {
				t.Fatal(err)
			}
			reencoded, err := merkle.Marshal()
			if err != nil || !bytes.Equal(proof, reencoded) {
				t.Fatal("IBC Go MerkleProof bytes differ from actual TS encoding")
			}
			key, _ := hex.DecodeString(job.Key)
			rebuilt, err := newProofJob(store.identity, id, job.LightClient, job.ProofHeight, key, merkle.Proofs[0].GetExist().Value, proof)
			if err != nil || !sameProof(job, rebuilt) {
				t.Fatalf("rebuilt legacy proof differs: %v", err)
			}
			if err := store.saveProof(job); err != nil {
				t.Fatal(err)
			}
			ids, err := store.proofJobIDs()
			if err != nil || len(ids) != 1 || ids[0] != id {
				t.Fatalf("proof IDs %v %v", ids, err)
			}
			job.Uploaded = 1
			if !sameProof(job, rebuilt) {
				t.Fatal("upload progress changes proof identity")
			}
			job.ProofHash = strings.Repeat("00", 32)
			if sameProof(job, rebuilt) {
				t.Fatal("proofHash ignored by sameProof")
			}
		})
	}
	wrongPath := filepath.Join(store.jobDirectory, strings.Repeat("00", 32)+".json.proof")
	if err := os.Rename(path, wrongPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.proofJobIDs(); err == nil {
		t.Fatal("misnamed proof accepted")
	}
}

func TestDurableProofRejectsCorruption(t *testing.T) {
	store := durableStore(t)
	id, _ := native.ParseID(durableIdentity(t).ID)
	data := durableFixture(t, "ts-proof-transfer.json")
	mutations := map[string]func(map[string]json.RawMessage){
		"extra":              func(v map[string]json.RawMessage) { v["future"] = json.RawMessage(`true`) },
		"identity":           func(v map[string]json.RawMessage) { v["genesis"] = json.RawMessage(`"wrong"`) },
		"missing zero field": func(v map[string]json.RawMessage) { delete(v, "uploaded") },
		"null zero field":    func(v map[string]json.RawMessage) { v["uploaded"] = json.RawMessage(`null`) },
		"predates":           func(v map[string]json.RawMessage) { v["proofHeight"] = json.RawMessage(`10`) },
		"unsafe height":      func(v map[string]json.RawMessage) { v["proofHeight"] = json.RawMessage(`9007199254740992`) },
		"progress":           func(v map[string]json.RawMessage) { v["uploaded"] = json.RawMessage(`16384`) },
		"allocation":         func(v map[string]json.RawMessage) { v["allocated"] = json.RawMessage(`16465`) },
		"rent":               func(v map[string]json.RawMessage) { v["lamports"] = json.RawMessage(`-1`) },
		"hash": func(v map[string]json.RawMessage) {
			v["valueHash"] = json.RawMessage(`"` + strings.Repeat("00", 32) + `"`)
		},
		"uppercase": func(v map[string]json.RawMessage) { v["nonce"] = json.RawMessage(strings.ToUpper(string(v["nonce"]))) },
		"wrong key valid hash": func(v map[string]json.RawMessage) {
			v["key"] = json.RawMessage(`"00"`)
			v["nonce"] = json.RawMessage(`"` + digestHex([]byte{0}) + `"`)
		},
		"bad proof valid hash": func(v map[string]json.RawMessage) {
			v["proof"] = json.RawMessage(`"00"`)
			v["proofHash"] = json.RawMessage(`"` + digestHex([]byte{0}) + `"`)
		},
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			mutation(fields)
			bad, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseProofJob(bad, store.identity, id); err == nil {
				t.Fatal("corrupted proof job accepted")
			}
		})
	}
	job, err := parseProofJob(data, store.identity, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := hex.DecodeString(job.Proof)
	var merkle commitment.MerkleProof
	if err := merkle.Unmarshal(raw); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(){
		func() { merkle.Proofs[1].GetExist().Key = []byte("bank") },
		func() { merkle.Proofs = merkle.Proofs[:1] },
	} {
		change()
		raw, err := merkle.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		job.Proof = hex.EncodeToString(raw)
		job.ProofHash = digestHex(raw)
		if err := job.validate(store.identity, id); err == nil {
			t.Fatal("invalid proof levels/store accepted")
		}
	}
}

func TestDurableProofAssertionMessagesAndOrder(t *testing.T) {
	store := durableStore(t)
	id, _ := native.ParseID(durableIdentity(t).ID)
	cases := []struct {
		name    string
		message string
		change  func(*proofJob)
		proof   func(*commitment.MerkleProof)
	}{
		{
			name:    "deployment",
			message: "proof job belongs to another deployment",
			change:  func(job *proofJob) { job.Genesis = "another-deployment" },
		},
		{
			name:    "identity",
			message: "proof job identity mismatch",
			change: func(job *proofJob) {
				other := id
				other.Sequence++
				job.ID = other.String()
			},
		},
		{
			name:    "nonce precedes stale height and proof hash",
			message: "proof job nonce mismatch",
			change: func(job *proofJob) {
				job.Nonce = strings.Repeat("00", 32)
				job.ProofHash = strings.Repeat("00", 32)
				job.ProofHeight = id.Height
			},
		},
		{
			name:    "proof hash precedes stale height",
			message: "proof job bytes changed",
			change: func(job *proofJob) {
				job.ProofHash = strings.Repeat("00", 32)
				job.ProofHeight = id.Height
			},
		},
		{
			name:    "size precedes stale height",
			message: "proof exceeds upload bound",
			change: func(job *proofJob) {
				proof := make([]byte, maxProofSize+1)
				job.Proof = hex.EncodeToString(proof)
				job.ProofHash = digestHex(proof)
				job.ProofHeight = id.Height
			},
		},
		{
			name:    "height",
			message: "proof job predates transfer",
			change:  func(job *proofJob) { job.ProofHeight = id.Height },
		},
		{
			name:    "key",
			message: "proof job key mismatch",
			change: func(job *proofJob) {
				job.Key = "00"
				job.Nonce = digestHex([]byte{0})
			},
		},
		{
			name:    "proof levels",
			message: "proof job needs two proof levels",
			proof:   func(proof *commitment.MerkleProof) { proof.Proofs = proof.Proofs[:1] },
		},
		{
			name:    "existence",
			message: "proof job needs existence values",
			proof:   func(proof *commitment.MerkleProof) { proof.Proofs[0].Proof = nil },
		},
		{
			name:    "existence key precedes store and value",
			message: "proof job existence key mismatch",
			proof: func(proof *commitment.MerkleProof) {
				proof.Proofs[0].GetExist().Key = []byte("wrong-key")
				proof.Proofs[1].GetExist().Key = []byte("bank")
				proof.Proofs[0].GetExist().Value = []byte("wrong-value")
			},
		},
		{
			name:    "store precedes value",
			message: "proof job store mismatch",
			proof: func(proof *commitment.MerkleProof) {
				proof.Proofs[1].GetExist().Key = []byte("bank")
				proof.Proofs[0].GetExist().Value = []byte("wrong-value")
			},
		},
		{
			name:    "value precedes upload progress",
			message: "proof job value hash mismatch",
			change: func(job *proofJob) {
				job.ValueHash = strings.Repeat("00", 32)
				job.Uploaded = maxProofSize
			},
		},
		{
			name:    "upload progress",
			message: "invalid upload progress",
			change:  func(job *proofJob) { job.Uploaded = maxProofSize },
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var job proofJob
			if err := json.Unmarshal(durableFixture(t, "ts-proof-transfer.json"), &job); err != nil {
				t.Fatal(err)
			}
			if test.proof != nil {
				raw, err := hex.DecodeString(job.Proof)
				if err != nil {
					t.Fatal(err)
				}
				var proof commitment.MerkleProof
				if err := proof.Unmarshal(raw); err != nil {
					t.Fatal(err)
				}
				test.proof(&proof)
				raw, err = proof.Marshal()
				if err != nil {
					t.Fatal(err)
				}
				job.Proof = hex.EncodeToString(raw)
				job.ProofHash = digestHex(raw)
			}
			if test.change != nil {
				test.change(&job)
			}
			encoded, err := json.Marshal(job)
			if err != nil {
				t.Fatal(err)
			}
			_, err = parseProofJob(encoded, store.identity, id)
			if err == nil || err.Error() != test.message {
				t.Fatalf("got %v, want %q", err, test.message)
			}
		})
	}
}
