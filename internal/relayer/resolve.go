package relayer

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

var errBudgetExhausted = errors.New("transfer is over its spend limit")

type budgetExhaustedError struct{ id sink.ID }

func (e budgetExhaustedError) Error() string {
	return fmt.Sprintf("transfer %s is over its spend limit", e.id.String())
}

func (e budgetExhaustedError) Unwrap() error { return errBudgetExhausted }

func (p *payerSession) resumePayment(ctx context.Context, id sink.ID) error {
	path, err := p.service.store.paymentPath(id)
	if err != nil {
		return err
	}
	_, err = p.resume(ctx, path, func(result txResult) error {
		return p.chargeResult(ctx, id, result)
	})
	return err
}

func (p *payerSession) paid(ctx context.Context, id sink.ID, stage string, instructions []solana.Instruction, reclaims bool) (string, error) {
	if err := p.resumePayment(ctx, id); err != nil {
		return "", err
	}
	s := p.service
	ledger, err := s.store.readLedger()
	if err != nil {
		return "", err
	}
	if !reclaims && ledger.exhausted(id, s.cfg.maxLamports) {
		return "", budgetExhaustedError{id: id}
	}
	path, err := s.store.paymentPath(id)
	if err != nil {
		return "", err
	}
	return timed(s, stage, &id, func() (string, error) {
		result, err := p.send(ctx, txRequest{
			stage:        stage,
			instructions: instructions,
			preflight:    true,
		}, path, func(result txResult) error {
			return p.chargeResult(ctx, id, result)
		})
		if err != nil {
			return "", err
		}
		if transactionFailed(result) {
			return "", fmt.Errorf("%s failed: %s (%s)", stage, result.failure, result.signature)
		}
		return result.signature, nil
	})
}

// These raw probes preserve the baseline's owner and empty-account decisions
// before preparation; typed reads validate initialized accounts separately.
func (s *service) resolutionAccount(ctx context.Context, address solana.PublicKey, commitment rpc.CommitmentType) (*rpc.Account, error) {
	account, _, err := s.reader.Account(ctx, address, commitment, nil)
	return account, err
}

func resolutionClient(f bridge.Followed, state bridge.ClientState) bridge.Followed {
	f.Revision = state.LatestHeight.RevisionNumber
	f.Latest = state.LatestHeight.RevisionHeight
	f.TrustNumerator = state.TrustLevelNumerator
	f.TrustDenominator = state.TrustLevelDenominator
	f.TrustingPeriod = state.TrustingPeriod
	f.UnbondingPeriod = state.UnbondingPeriod
	f.MaxClockDrift = state.MaxClockDrift
	f.FrozenRevision = state.FrozenHeight.RevisionNumber
	f.FrozenHeight = state.FrozenHeight.RevisionHeight
	return f
}

func (p *payerSession) relayWith(ctx context.Context, followed bridge.Followed, id sink.ID, height headerHeight, prepared *preparedProof) (string, error) {
	s := p.service
	if err := p.resumePayment(ctx, id); err != nil {
		return "", err
	}
	address, err := s.deployment.Status(id)
	if err != nil {
		return "", err
	}
	status, err := s.resolutionAccount(ctx, address, rpc.CommitmentConfirmed)
	if err != nil {
		return "", err
	}
	if status != nil && status.Data == nil {
		return "", fmt.Errorf("status account data unavailable")
	}
	if status != nil && status.Owner == s.cfg.program && len(status.Data.GetBinary()) > 8 {
		resolved, err := s.reader.Status(ctx, id)
		if err != nil {
			return "", err
		}
		if resolved == nil {
			return "", fmt.Errorf("status account unavailable")
		}
		if err := p.finishProofBuffers(ctx, id); err != nil {
			return "", err
		}
		return "already-resolved", nil
	}
	ledger, err := s.store.readLedger()
	if err != nil {
		return "", err
	}
	if ledger.exhausted(id, s.cfg.maxLamports) {
		return "skipped", nil
	}
	if prepared == nil {
		proof, err := s.prepareProof(ctx, id, height)
		prepared = &preparedProof{
			proof: proof,
			err:   err,
		}
	}
	if prepared.err != nil {
		return "", prepared.err
	}
	proof := prepared.proof
	latest, err := timed(s, "state-query", &id, func() (native.Snapshot, error) {
		return s.native.Snapshot(ctx)
	})
	if err != nil {
		return "", err
	}
	now, err := timed(s, "transfer-query", &id, func() (sink.TransferState, error) {
		return s.native.Transfer(ctx, id, latest.Height)
	})
	if err != nil {
		return "", err
	}
	if changed := finalCheck(proof.Transfer, now); changed != "" {
		s.emit(true, map[string]any{
			"event":       "relay-final-check",
			"id":          id.String(),
			"reason":      changed,
			"akashStatus": now.Status.String(),
			"akashHeight": latest.Height,
			"action":      "skipped",
		})
		return "skipped", nil
	}
	if _, err := s.store.updateLedger(ctx, func(ledger *spendLedger) (bool, error) {
		return true, ledger.attempt(id)
	}); err != nil {
		return "", err
	}
	job, err := newProofJob(s.store.identity, id, followed.Program.String(), int64(height), proof.Key, proof.Value, proof.Merkle)
	if err != nil {
		return "", err
	}
	buffer, err := p.uploadProof(ctx, id, job)
	if err != nil {
		return "", err
	}

	// Upload can span many blocks. Recheck the outcome before resolution.
	finalState, err := timed(s, "state-query", &id, func() (native.Snapshot, error) {
		return s.native.Snapshot(ctx)
	})
	if err != nil {
		return "", err
	}
	finalTransfer, err := timed(s, "transfer-query", &id, func() (sink.TransferState, error) {
		return s.native.Transfer(ctx, id, finalState.Height)
	})
	if err != nil {
		return "", err
	}
	if finalCheck(proof.Transfer, finalTransfer) != "" {
		if err := p.closeProofBuffer(ctx, id); err != nil {
			return "", err
		}
		return "skipped", nil
	}
	resolved, err := s.reader.Status(ctx, id)
	if err != nil {
		return "", err
	}
	if resolved != nil {
		if err := p.finishProofBuffers(ctx, id); err != nil {
			return "", err
		}
		return "already-resolved", nil
	}
	client, err := s.reader.Client(ctx, followed)
	if err != nil {
		return "", err
	}
	followed = resolutionClient(followed, client)
	var resolution solana.Instruction
	if proof.Transfer.Settlement.VoidAuthorization != nil {
		resolution, err = s.deployment.VoidTransfer(bridge.VoidArgs{
			Payer:            s.payer.PublicKey(),
			Client:           followed,
			ID:               id,
			ProofBuffer:      buffer,
			SettlementStatus: uint8(proof.Transfer.Status),
			ProofHeight:      uint64(height),
		})
	} else {
		recipient, parseErr := solana.PublicKeyFromBase58(proof.Transfer.Data.Dest)
		if parseErr != nil {
			return "", parseErr
		}
		resolution, err = s.deployment.MintTransfer(bridge.MintArgs{
			Payer:       s.payer.PublicKey(),
			Client:      followed,
			ID:          id,
			ProofBuffer: buffer,
			Recipient:   recipient,
			Amount:      proof.Transfer.Data.Amount.Amount.Uint64(),
			ProofHeight: uint64(height),
		})
	}
	if err != nil {
		return "", err
	}
	signature, err := p.paid(ctx, id, "resolve", append(bridge.Compute(), resolution), false)
	if err != nil {
		return "", err
	}
	if err := p.finishProofBuffers(ctx, id); err != nil {
		return "", err
	}
	ledger, err = s.store.readLedger()
	if err != nil {
		return "", err
	}
	s.emit(false, map[string]any{
		"event":         "transfer-resolved",
		"id":            id.String(),
		"proofHeight":   height,
		"proofBytes":    len(proof.Merkle),
		"signature":     signature,
		"spentLamports": ledger.spent(id),
		"attempts":      ledger.entries[id].Attempts,
	})
	return "resolved", nil
}

func (s *service) relay(ctx context.Context, id sink.ID, height headerHeight) (string, error) {
	result := "skipped"
	err := s.withRequiredPayer(ctx, func(p *payerSession) error {
		followed, err := s.follow(ctx)
		if err != nil {
			return err
		}
		result, err = p.relayWith(ctx, followed, id, height, nil)
		return err
	})
	return result, err
}
