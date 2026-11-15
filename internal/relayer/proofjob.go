package relayer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/native"
	"pkg.akt.dev/node/v2/x/sink/keeper"
)

const (
	maxProofSize    = 16 * 1024
	proofHeaderSize = 80
)

type proofIdentity struct {
	ChainID   string
	Genesis   string
	ProgramID string
	Payer     string
}

type proofJob struct {
	Version     int    `json:"version"`
	ChainID     string `json:"chainId"`
	Genesis     string `json:"genesis"`
	ProgramID   string `json:"programId"`
	Payer       string `json:"payer"`
	ID          string `json:"id"`
	LightClient string `json:"lightClient"`
	ProofHeight int64  `json:"proofHeight"`
	Key         string `json:"key"`
	Nonce       string `json:"nonce"`
	ValueHash   string `json:"valueHash"`
	ProofHash   string `json:"proofHash"`
	Proof       string `json:"proof"`
	Uploaded    int    `json:"uploaded"`
	Allocated   int    `json:"allocated"`
	Lamports    int64  `json:"lamports"`
}

func digestHex(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func newProofJob(identity proofIdentity, id sink.ID, client string, height int64, key, value, proof []byte) (*proofJob, error) {
	job := &proofJob{
		Version:     1,
		ChainID:     identity.ChainID,
		Genesis:     identity.Genesis,
		ProgramID:   identity.ProgramID,
		Payer:       identity.Payer,
		ID:          id.String(),
		LightClient: client,
		ProofHeight: height,
		Key:         hex.EncodeToString(key),
		Nonce:       digestHex(key),
		ValueHash:   digestHex(value),
		ProofHash:   digestHex(proof),
		Proof:       hex.EncodeToString(proof),
	}
	if err := job.validate(identity, id); err != nil {
		return nil, err
	}
	return job, nil
}

func parseProofJob(data []byte, identity proofIdentity, id sink.ID) (*proofJob, error) {
	fields, err := jsonObject(data)
	if err != nil {
		return nil, err
	}
	var job proofJob
	names := documentFields(job)
	if err := requireFields(fields, names...); err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[name] = true
	}
	for name := range fields {
		if !known[name] {
			return nil, fmt.Errorf("unknown proof job field %s", name)
		}
	}
	if err := decodeExactFields(fields, &job, "version", "proofHeight", "uploaded", "allocated", "lamports"); err != nil {
		return nil, err
	}
	if err := job.validate(identity, id); err != nil {
		return nil, err
	}
	return &job, nil
}

func (job *proofJob) validate(identity proofIdentity, id sink.ID) error {
	if job.Version != 1 || job.ChainID == "" || job.Genesis == "" || job.ProgramID == "" || job.Payer == "" {
		return fmt.Errorf("invalid proof job version or deployment")
	}
	parsed, err := native.ParseID(job.ID)
	if err != nil {
		return err
	}
	if job.LightClient == "" || !positiveSafe(job.ProofHeight) {
		return fmt.Errorf("invalid proof job height or client")
	}
	for _, value := range []string{job.Key, job.Proof} {
		if value == "" || !validHex(value, -1, true) {
			return fmt.Errorf("invalid proof job hexadecimal fields")
		}
	}
	for _, value := range []string{job.Nonce, job.ValueHash, job.ProofHash} {
		if !validHex(value, 32, true) {
			return fmt.Errorf("invalid proof job hexadecimal fields")
		}
	}
	if job.Uploaded < 0 || job.Uploaded > maxProofSize {
		return fmt.Errorf("invalid proof job uploaded bound")
	}
	if job.Allocated < 0 || job.Allocated > proofHeaderSize+maxProofSize {
		return fmt.Errorf("invalid proof job allocation")
	}
	if job.Lamports < 0 || !safeInteger(job.Lamports) {
		return fmt.Errorf("invalid proof job lamports")
	}
	if job.ChainID != identity.ChainID || job.Genesis != identity.Genesis || job.ProgramID != identity.ProgramID || job.Payer != identity.Payer {
		return fmt.Errorf("proof job belongs to another deployment")
	}
	if parsed != id {
		return fmt.Errorf("proof job identity mismatch")
	}
	key, _ := hex.DecodeString(job.Key)
	proof, _ := hex.DecodeString(job.Proof)
	if digestHex(key) != job.Nonce {
		return fmt.Errorf("proof job nonce mismatch")
	}
	if digestHex(proof) != job.ProofHash {
		return fmt.Errorf("proof job bytes changed")
	}
	if len(proof) > maxProofSize {
		return fmt.Errorf("proof exceeds upload bound")
	}
	if job.ProofHeight <= id.Height {
		return fmt.Errorf("proof job predates transfer")
	}
	expected, err := keeper.TransferKey(id)
	if err != nil {
		return err
	}
	matches := bytes.Equal(key, expected)
	for status := sink.StatusTransferPending; !matches && status <= sink.StatusRefunded; status++ {
		expected, err = keeper.SettlementKey(sink.SettlementID{
			Status:   status,
			Owner:    id.Owner,
			Height:   id.Height,
			Sequence: id.Sequence,
		})
		if err != nil {
			return err
		}
		matches = bytes.Equal(key, expected)
	}
	if !matches {
		return fmt.Errorf("proof job key mismatch")
	}
	var decoded commitment.MerkleProof
	if err := decoded.Unmarshal(proof); err != nil {
		return fmt.Errorf("decode proof job: %w", err)
	}
	if len(decoded.Proofs) != 2 || decoded.Proofs[0] == nil || decoded.Proofs[1] == nil {
		return fmt.Errorf("proof job needs two proof levels")
	}
	iavl, store := decoded.Proofs[0].GetExist(), decoded.Proofs[1].GetExist()
	if iavl == nil || store == nil {
		return fmt.Errorf("proof job needs existence values")
	}
	if !bytes.Equal(iavl.Key, key) {
		return fmt.Errorf("proof job existence key mismatch")
	}
	if !bytes.Equal(store.Key, []byte("sink")) {
		return fmt.Errorf("proof job store mismatch")
	}
	if digestHex(iavl.Value) != job.ValueHash {
		return fmt.Errorf("proof job value hash mismatch")
	}
	if job.Uploaded > len(proof) {
		return fmt.Errorf("invalid upload progress")
	}
	return nil
}

func sameProof(a, b *proofJob) bool {
	return a != nil && b != nil && a.Key == b.Key && a.ValueHash == b.ValueHash && a.ProofHash == b.ProofHash && a.ProofHeight == b.ProofHeight && a.LightClient == b.LightClient
}
