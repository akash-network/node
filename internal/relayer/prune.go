package relayer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	solana "github.com/gagliardetto/solana-go"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

type consensusPruneResult struct {
	LatestHeight      uint64   `json:"latestHeight"`
	Scanned           int      `json:"scanned"`
	Expired           int      `json:"expired"`
	Pruned            int      `json:"pruned"`
	Failed            int      `json:"failed"`
	Remaining         int      `json:"remaining"`
	ReclaimedLamports uint64   `json:"reclaimedLamports"`
	FeeLamports       uint64   `json:"feeLamports"`
	Signatures        []string `json:"signatures"`
	Skipped           string   `json:"skipped,omitempty"`
}

func (s *service) pruneConsensus(ctx context.Context, followed bridge.Followed) error {
	now := s.deps.now().UnixMilli()
	if now-s.prunedAt < int64(s.cfg.pruneIntervalSeconds)*1000 {
		return nil
	}
	s.prunedAt = now
	err := s.withRequiredPayer(ctx, func(p *payerSession) error {
		result, err := p.pruneConsensus(ctx, followed, 20, 8)
		if err != nil {
			return err
		}
		if result.Pruned != 0 || result.Failed != 0 {
			s.emit(false, map[string]any{
				"event":             "consensus-pruned",
				"latestHeight":      result.LatestHeight,
				"scanned":           result.Scanned,
				"expired":           result.Expired,
				"pruned":            result.Pruned,
				"failed":            result.Failed,
				"remaining":         result.Remaining,
				"reclaimedLamports": result.ReclaimedLamports,
				"feeLamports":       result.FeeLamports,
				"signatures":        result.Signatures,
			})
		}
		return nil
	})
	if err != nil {
		s.emit(true, map[string]any{
			"event": "consensus-prune-failed",
			"error": err.Error(),
		})
	}
	return nil
}

func (p *payerSession) pruneConsensus(ctx context.Context, f bridge.Followed, maxPerTick, perTransaction int) (consensusPruneResult, error) {
	result := consensusPruneResult{
		Signatures: []string{},
	}
	if maxPerTick <= 0 || perTransaction <= 0 || perTransaction > 10 {
		return result, fmt.Errorf("invalid consensus prune limits")
	}
	s := p.service
	state, err := s.reader.Client(ctx, f)
	if err != nil {
		return result, err
	}
	result.LatestHeight = state.LatestHeight.RevisionHeight
	access, err := s.reader.Access(ctx, f, s.payer.PublicKey())
	if err != nil {
		return result, err
	}
	if access != bridge.AccessMember {
		result.Skipped = "payer lacks RELAYER_ROLE"
		return result, nil
	}
	now, err := s.reader.Clock(ctx)
	if err != nil {
		return result, err
	}
	accounts, err := s.reader.ConsensusStates(ctx, f)
	if err != nil {
		return result, err
	}
	result.Scanned = len(accounts)
	candidates := make([]bridge.Consensus, 0, len(accounts))
	for _, entry := range accounts {
		address, err := f.ConsensusAt(entry.Height)
		if err != nil {
			return result, err
		}
		if entry.Height != result.LatestHeight && bridge.Expired(entry.Timestamp, state.TrustingPeriod, now) && entry.Address == address {
			candidates = append(candidates, entry)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Height < candidates[j].Height })
	result.Expired = len(candidates)
	selected := candidates[:min(len(candidates), maxPerTick)]
	for i := 0; i < len(selected); i += perTransaction {
		batch := selected[i:min(len(selected), i+perTransaction)]
		instructions := make([]solana.Instruction, len(batch))
		for j, entry := range batch {
			ix, err := f.Prune(s.payer.PublicKey(), entry.Height)
			if err != nil {
				return result, err
			}
			instructions[j] = ix
		}
		sent, err := p.sendPrune(ctx, instructions)
		if err != nil && !strings.HasPrefix(err.Error(), "consensus-prune: ") {
			return result, err
		}
		if err == nil {
			result.Signatures = append(result.Signatures, sent.signature)
			result.FeeLamports += sent.fee
		}
		if err != nil || transactionFailed(sent) {
			result.Failed += len(batch)
			break
		}
		result.Pruned += len(batch)
		for _, entry := range batch {
			result.ReclaimedLamports += entry.Lamports
		}
	}
	result.Remaining = result.Expired - result.Pruned
	return result, nil
}
