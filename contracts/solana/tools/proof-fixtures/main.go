// Generates local IAVL + multistore ICS23 proofs. This does not create a signed
// Akash header; tests explicitly initialize a trusted checkpoint with this root.
package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/iavl"
	"github.com/cosmos/iavl/db"
	ics23 "github.com/cosmos/ics23/go"
)

// Entry is one sink store write. Key and Value are hex-encoded bytes.
type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type ProvenEntry struct {
	Entry
	Proof string `json:"proof"`
}
type Fixture struct {
	Root    string        `json:"root"`
	Entries []ProvenEntry `json:"entries"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var entries []Entry
	decoder := json.NewDecoder(os.Stdin)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return err
	}
	tree := iavl.NewMutableTree(db.NewMemDB(), 0, true, iavl.NewNopLogger())
	// Tree depth ensures the upload exercises multiple Solana transactions.
	// Padding records use transfer keys (0x02 0x00, then a big-endian u64 ID)
	// with IDs from 1,000,000, clear of the test IDs.
	for i := uint64(0); i < 1024; i++ {
		key := binary.BigEndian.AppendUint64([]byte{0x02, 0x00}, 1_000_000+i)
		if _, err := tree.Set(key, []byte{1}); err != nil {
			return err
		}
	}
	keys := make([][]byte, len(entries))
	values := make([][]byte, len(entries))
	for i, entry := range entries {
		var err error
		if keys[i], err = hex.DecodeString(entry.Key); err != nil {
			return fmt.Errorf("entry %d key: %w", i, err)
		}
		if values[i], err = hex.DecodeString(entry.Value); err != nil {
			return fmt.Errorf("entry %d value: %w", i, err)
		}
		if _, err = tree.Set(keys[i], values[i]); err != nil {
			return err
		}
	}
	root, _, err := tree.SaveVersion()
	if err != nil {
		return err
	}
	storeProof := &ics23.CommitmentProof{Proof: &ics23.CommitmentProof_Exist{Exist: &ics23.ExistenceProof{
		Key: []byte("sink"), Value: root, Leaf: ics23.TendermintSpec.LeafSpec,
	}}}
	appRoot, err := storeProof.Calculate()
	if err != nil {
		return err
	}
	storeBytes, err := storeProof.Marshal()
	if err != nil {
		return err
	}
	fixture := Fixture{Root: hex.EncodeToString(appRoot)}
	for i, entry := range entries {
		proof, err := tree.GetMembershipProof(keys[i])
		if err != nil {
			return err
		}
		if !ics23.VerifyMembership(ics23.IavlSpec, root, proof, keys[i], values[i]) ||
			!ics23.VerifyMembership(ics23.TendermintSpec, appRoot, storeProof, []byte("sink"), root) {
			return fmt.Errorf("generated proof did not verify: %s", entry.Key)
		}
		proofBytes, err := proof.Marshal()
		if err != nil {
			return err
		}
		// ibc.core.commitment.v1.MerkleProof has repeated CommitmentProof proofs = 1.
		var merkleProof []byte
		for _, part := range [][]byte{proofBytes, storeBytes} {
			merkleProof = append(merkleProof, 0x0a)
			merkleProof = binary.AppendUvarint(merkleProof, uint64(len(part)))
			merkleProof = append(merkleProof, part...)
		}
		fixture.Entries = append(fixture.Entries, ProvenEntry{Entry: entry, Proof: hex.EncodeToString(merkleProof)})
	}
	return json.NewEncoder(os.Stdout).Encode(fixture)
}
