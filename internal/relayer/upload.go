package relayer

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"github.com/gagliardetto/solana-go"

	sink "pkg.akt.dev/go/node/sink/v1"
)

func (p *payerSession) closeProofBuffer(ctx context.Context, id sink.ID) error {
	if err := p.resumePayment(ctx, id); err != nil {
		return err
	}
	s := p.service
	job, err := s.store.loadProof(id)
	if err != nil || job == nil {
		return err
	}

	key, err := hex.DecodeString(job.Key)
	if err != nil {
		return err
	}
	address, err := s.deployment.ProofBuffer(s.payer.PublicKey(), key)
	if err != nil {
		return err
	}

	account, err := s.reader.BufferConfirmed(ctx, address)
	if err != nil {
		return err
	}
	if account != nil {
		if account.Submitter != s.payer.PublicKey() {
			return fmt.Errorf("proof buffer submitter mismatch")
		}
		if hex.EncodeToString(account.Nonce[:]) != job.Nonce {
			return fmt.Errorf("proof buffer nonce mismatch")
		}
		instruction, err := s.deployment.CloseProofBuffer(s.payer.PublicKey(), address)
		if err != nil {
			return err
		}
		if _, err := p.paid(ctx, id, "proof-close", []solana.Instruction{instruction}, true); err != nil {
			return err
		}
	}

	return s.store.removeProof(id)
}

func (p *payerSession) finishProofBuffers(ctx context.Context, id sink.ID) error {
	if err := p.closeProofBuffer(ctx, id); err != nil {
		return err
	}

	_, err := p.service.store.updateLedger(ctx, func(ledger *spendLedger) (bool, error) {
		ledger.complete(id)
		return true, nil
	})
	return err
}

func (p *payerSession) uploadProof(ctx context.Context, id sink.ID, wanted *proofJob) (solana.PublicKey, error) {
	s := p.service
	job, err := s.store.loadProof(id)
	if err != nil {
		return solana.PublicKey{}, err
	}
	if job != nil && !sameProof(job, wanted) {
		if err := p.closeProofBuffer(ctx, id); err != nil {
			return solana.PublicKey{}, err
		}
		job = nil
	}
	if job == nil {
		job = wanted
		if err := s.store.saveProof(job); err != nil {
			return solana.PublicKey{}, err
		}
	}

	proof, err := hex.DecodeString(job.Proof)
	if err != nil {
		return solana.PublicKey{}, err
	}

	key, err := hex.DecodeString(job.Key)
	if err != nil {
		return solana.PublicKey{}, err
	}
	address, err := s.deployment.ProofBuffer(s.payer.PublicKey(), key)
	if err != nil {
		return solana.PublicKey{}, err
	}

	progress := func() (*proofJob, error) {
		account, err := s.reader.Buffer(ctx, address)
		if err != nil || account == nil {
			return nil, err
		}
		if account.Submitter != s.payer.PublicKey() {
			return nil, fmt.Errorf("proof buffer submitter mismatch")
		}
		if hex.EncodeToString(account.Nonce[:]) != wanted.Nonce {
			return nil, fmt.Errorf("proof buffer nonce mismatch")
		}
		if uint64(account.ExpectedLen) != uint64(len(proof)) {
			return nil, fmt.Errorf("proof buffer length mismatch")
		}
		if len(account.Data) > len(proof) {
			return nil, fmt.Errorf("proof buffer exceeds expected length")
		}
		if !bytes.Equal(account.Data, proof[:len(account.Data)]) {
			return nil, fmt.Errorf("proof upload prefix mismatch")
		}
		if account.Lamports > maxSafeInteger {
			return nil, fmt.Errorf("proof buffer lamports exceed safe integer")
		}

		updated := *wanted
		updated.Uploaded = len(account.Data)
		updated.Allocated = account.Allocated
		updated.Lamports = int64(account.Lamports)
		if err := s.store.saveProof(&updated); err != nil {
			return nil, err
		}
		return &updated, nil
	}

	state, err := progress()
	if err != nil {
		return solana.PublicKey{}, err
	}
	if state == nil {
		instruction, err := s.deployment.CreateProofBuffer(s.payer.PublicKey(), key, uint32(len(proof)))
		if err != nil {
			return solana.PublicKey{}, err
		}
		if _, err := p.paid(ctx, id, "proof-create", []solana.Instruction{instruction}, false); err != nil {
			return solana.PublicKey{}, err
		}
		state, err = progress()
		if err != nil {
			return solana.PublicKey{}, err
		}
		if state == nil {
			return solana.PublicKey{}, fmt.Errorf("created proof buffer missing")
		}
	}

	for state.Allocated < proofHeaderSize+len(proof) {
		previous := state.Allocated
		instruction, err := s.deployment.GrowProofBuffer(s.payer.PublicKey(), address)
		if err != nil {
			return solana.PublicKey{}, err
		}
		if _, err := p.paid(ctx, id, "proof-grow", []solana.Instruction{instruction}, false); err != nil {
			return solana.PublicKey{}, err
		}
		state, err = progress()
		if err != nil {
			return solana.PublicKey{}, err
		}
		if state == nil || state.Allocated <= previous {
			return solana.PublicKey{}, fmt.Errorf("proof buffer did not grow")
		}
	}

	for state.Uploaded < len(proof) {
		offset := state.Uploaded
		const chunkSize = 450
		end := min(offset+2*chunkSize, len(proof))
		instructions := make([]solana.Instruction, 0, 2)
		for start := offset; start < end; start += chunkSize {
			instruction, err := s.deployment.AppendProof(s.payer.PublicKey(), address, uint32(start), proof[start:min(start+chunkSize, end)])
			if err != nil {
				return solana.PublicKey{}, err
			}
			instructions = append(instructions, instruction)
		}
		if _, err := p.paid(ctx, id, "proof-append", instructions, false); err != nil {
			return solana.PublicKey{}, err
		}
		state, err = progress()
		if err != nil {
			return solana.PublicKey{}, err
		}
		if state == nil || state.Uploaded <= offset {
			return solana.PublicKey{}, fmt.Errorf("proof upload did not advance")
		}
	}

	s.emit(false, map[string]any{
		"event":              "proof-buffer-ready",
		"id":                 id.String(),
		"key":                job.Key,
		"nonce":              job.Nonce,
		"proofHeight":        job.ProofHeight,
		"bytes":              len(proof),
		"allocatedBytes":     state.Allocated,
		"refundableLamports": state.Lamports,
	})
	return address, nil
}
