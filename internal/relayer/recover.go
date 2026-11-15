package relayer

import (
	"context"
	"encoding/hex"

	"github.com/gagliardetto/solana-go/rpc"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func (s *service) recoverPayments(ctx context.Context) error {
	err := s.withRequiredPayer(ctx, func(p *payerSession) error {
		ledger, err := s.store.readLedger()
		if err != nil {
			return err
		}
		saved, err := s.store.proofJobIDs()
		if err != nil {
			return err
		}
		proofs := make(map[sink.ID]bool, len(saved))
		for _, id := range saved {
			proofs[id] = true
		}
		ids := ledger.ids()
		for _, id := range saved {
			if _, exists := ledger.entries[id]; !exists {
				ids = append(ids, id)
			}
		}
		for _, id := range ids {
			completed := ledger.entries[id].Completed
			if completed != nil && *completed && !proofs[id] {
				continue
			}
			if err := p.resumePayment(ctx, id); err != nil {
				return err
			}
			status, err := s.reader.Status(ctx, id)
			if err != nil {
				return err
			}
			if status != nil {
				if err := p.finishProofBuffers(ctx, id); err != nil {
					return err
				}
				continue
			}
			job, err := s.store.loadProof(id)
			if err != nil {
				return err
			}
			if job == nil {
				continue
			}
			state, err := s.native.Snapshot(ctx)
			if err != nil {
				return err
			}
			transfer, err := s.native.Transfer(ctx, id, state.Height)
			if err != nil {
				return err
			}
			key, value, err := native.ProofValue(transfer, state.Height)
			if err != nil {
				return err
			}
			followed, err := s.follow(ctx)
			if err != nil {
				return err
			}
			address, err := followed.ConsensusAt(uint64(job.ProofHeight))
			if err != nil {
				return err
			}
			consensus, err := s.resolutionAccount(ctx, address, rpc.CommitmentFinalized)
			if err != nil {
				return err
			}
			if consensus == nil || consensus.Owner != followed.Program || job.Key != hex.EncodeToString(key) || job.ValueHash != digestHex(value) || job.LightClient != followed.Program.String() || unbackedMint(transfer) != "" {
				if err := p.closeProofBuffer(ctx, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return err
}

func (s *service) resetBudget(ctx context.Context, value string) error {
	if value != "all" {
		if _, err := native.ParseID(value); err != nil {
			return err
		}
	}
	err := s.withRequiredPayer(ctx, func(p *payerSession) error {
		ledger, err := s.store.readLedger()
		if err != nil {
			return err
		}
		for _, id := range ledger.ids() {
			if value == "all" || value == id.String() {
				if err := p.resumePayment(ctx, id); err != nil {
					return err
				}
			}
		}
		_, err = s.store.updateLedger(ctx, func(ledger *spendLedger) (bool, error) {
			return ledger.reset(value)
		})
		return err
	})
	if err != nil {
		return err
	}
	s.emit(false, map[string]any{
		"event": "relay-budget-reset",
		"id":    value,
		"path":  s.store.ledgerPath,
	})
	return nil
}
