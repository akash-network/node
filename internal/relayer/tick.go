package relayer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
)

func (s *service) relayFailed(ctx context.Context, id sink.ID, failure error) error {
	if isIntentNotIncluded(failure) {
		return failure
	}
	var failures, delay int64
	_, err := s.store.updateLedger(ctx, func(ledger *spendLedger) (bool, error) {
		var err error
		failures, delay, err = ledger.failed(id, s.deps.now().UnixMilli(), int64(s.cfg.retryMaxSeconds*1000))
		return true, err
	})
	if err != nil {
		return err
	}
	s.emit(true, map[string]any{
		"event":        "relay-retry",
		"id":           id.String(),
		"attempt":      failures,
		"retrySeconds": float64(delay) / 1000,
		"error":        failure.Error(),
	})
	return nil
}

func (s *service) overBudget(ctx context.Context, id sink.ID, ledger *spendLedger) error {
	if !s.alertDue("exhausted:" + id.String()) {
		return nil
	}
	s.emit(true, map[string]any{
		"event":                  "relay-budget-exhausted",
		"id":                     id.String(),
		"spentLamports":          ledger.spent(id),
		"maxLamportsPerTransfer": s.cfg.maxLamports,
		"attempts":               ledger.entries[id].Attempts,
		"ledger":                 s.store.ledgerPath,
		"action":                 "skipped",
	})
	if err := s.withRequiredPayer(ctx, func(p *payerSession) error {
		return p.closeProofBuffer(ctx, id)
	}); err != nil {
		if isIntentNotIncluded(err) {
			return err
		}
		s.emit(true, map[string]any{
			"event": "proof-buffer-close-failed",
			"id":    id.String(),
			"error": err.Error(),
		})
	}
	return nil
}

func advanceHeaderRetry(state *retryState, now, maximum int64, unsupported bool) (int64, error) {
	failures, err := safeSum(state.Failures, 1)
	if err != nil {
		return 0, err
	}
	delay := maximum
	if !unsupported {
		delay = min(maximum, int64(6000)<<min(failures-1, 30))
	}
	if now > math.MaxInt64-delay {
		return 0, fmt.Errorf("header retry clock overflow")
	}
	*state = retryState{
		Failures: failures,
		RetryAt:  now + delay,
	}
	return delay, nil
}

func (s *service) headerStep(action func() error, target headerHeight) (bool, error) {
	now := s.deps.now().UnixMilli()
	if now < s.headerRetry.RetryAt || now < s.unsupported.RetryAt {
		return false, nil
	}
	failure := action()
	if failure == nil {
		s.unsupported = retryState{}
		return true, nil
	}
	var admission *headerAdmissionError
	if errors.As(failure, &admission) || isIntentNotIncluded(failure) {
		return false, failure
	}
	fields := map[string]any{}
	if target != 0 {
		fields["target"] = target
	}
	if errors.Is(failure, header.ErrUnsupported) {
		delay, err := advanceHeaderRetry(&s.unsupported, s.deps.now().UnixMilli(), int64(s.cfg.retryMaxSeconds*1000), true)
		if err != nil {
			return false, err
		}
		fields["event"] = "header-unsupported"
		fields["reason"] = failure.Error()
		fields["retrySeconds"] = float64(delay) / 1000
	} else {
		delay, err := advanceHeaderRetry(&s.headerRetry, s.deps.now().UnixMilli(), 120_000, false)
		if err != nil {
			return false, err
		}
		fields["event"] = "header-retry"
		fields["attempt"] = s.headerRetry.Failures
		fields["retrySeconds"] = float64(delay) / 1000
		fields["error"] = failure.Error()
	}
	s.emit(true, fields)
	return false, nil
}

func (s *service) tick(ctx context.Context) error {
	return timedAction(s, "tick", nil, func() error {
		return s.tickWork(ctx)
	})
}

func (s *service) tickWork(ctx context.Context) error {
	followed, err := s.follow(ctx)
	if err != nil {
		return err
	}
	if err := s.recoverPayments(ctx); err != nil {
		return err
	}

	upkeep := &headerUpkeep{
		service:  s,
		expected: followed,
	}
	if err := upkeep.refresh(ctx, nil); err != nil {
		return err
	}

	// Every discovery page and plan uses this same committed native snapshot.
	state := upkeep.state
	pending, err := timed(s, "discovery", nil, func() (discovery, error) {
		return s.unresolved(ctx, state, upkeep.check)
	})
	if err != nil {
		return err
	}

	ledger, err := s.store.readLedger()
	if err != nil {
		return err
	}

	target := headerHeight(state.Height - 1)
	plans := make([]relayPlan, 0, len(pending.ids))
	for _, id := range pending.ids {
		if err := upkeep.check(ctx); err != nil {
			return err
		}
		if !ledger.ready(id, s.deps.now().UnixMilli()) {
			continue
		}
		if ledger.exhausted(id, s.cfg.maxLamports) {
			upkeep.paymentsDirty = true
			if err := s.overBudget(ctx, id, ledger); err != nil {
				return err
			}
			continue
		}

		s.clearAlert("exhausted:" + id.String())
		var known *sink.TransferState
		if transfer, exists := pending.known[id]; exists {
			known = &transfer
		}

		entry, err := s.plan(ctx, id, stateHeight(state.Height), upkeep.current.height, known)
		if err != nil {
			if err := s.relayFailed(ctx, id, err); err != nil {
				return err
			}
			continue
		}
		if entry.unbacked != "" {
			if s.alertDue("unbacked:" + id.String()) {
				s.emit(true, map[string]any{
					"event":       "relay-unbacked-settlement",
					"id":          id.String(),
					"akashStatus": entry.transfer.Status.String(),
					"reason":      entry.unbacked,
					"action":      "skipped",
				})
			}
			continue
		}

		s.clearAlert("unbacked:" + id.String())
		if entry.height <= target {
			plans = append(plans, entry)
		}
	}

	if err := upkeep.check(ctx); err != nil {
		return err
	}

	ready, err := s.payable(ctx, plans, upkeep.check)
	if err != nil {
		return err
	}

	required := make([]headerHeight, 0, len(ready))
	for _, entry := range ready {
		required = append(required, entry.height)
	}
	if err := upkeep.refresh(ctx, required); err != nil {
		return err
	}

	covered := make([]relayPlan, 0, len(ready))
	for _, entry := range ready {
		if entry.height <= upkeep.current.height {
			covered = append(covered, entry)
		}
	}
	covered = relayOrder(covered, s.payer.PublicKey())
	if err := s.relayCovered(ctx, covered, upkeep.header, func() { upkeep.paymentsDirty = true }); err != nil {
		return err
	}

	followed, err = s.follow(ctx)
	if err != nil {
		return err
	}
	if err := upkeep.validateFollow(followed); err != nil {
		return err
	}

	allowed, err := s.relayerAccess(ctx, followed)
	if err != nil {
		return err
	}
	if allowed == bridge.AccessMember {
		if err := s.pruneConsensus(ctx, followed); err != nil {
			return err
		}
	}
	return s.reclaimLookups(ctx)
}

func (s *service) relayCovered(ctx context.Context, covered []relayPlan, headers func(context.Context) (relayHeader, error), beforePayment func()) error {
	for offset := 0; offset < len(covered); offset += 2 {
		view, err := headers(ctx)
		if err != nil {
			return err
		}

		batch := covered[offset:min(offset+2, len(covered))]
		prepared := make([]preparedProof, len(batch))
		heights := make([]headerHeight, len(batch))
		var workers sync.WaitGroup

		for index, entry := range batch {
			height := view.height
			if entry.resumeHeight != 0 {
				height = entry.resumeHeight
			}
			heights[index] = height
			workers.Add(1)
			go func() {
				defer workers.Done()
				prepared[index].proof, prepared[index].err = s.prepareProof(ctx, entry.id, height)
			}()
		}

		workers.Wait()
		if _, err := headers(ctx); err != nil {
			return err
		}

		for index, entry := range batch {
			beforePayment()
			err := s.withRequiredPayer(ctx, func(p *payerSession) error {
				_, err := p.relayWith(ctx, view.followed, entry.id, heights[index], &prepared[index])
				return err
			})
			if err == nil {
				_, err = s.store.updateLedger(ctx, func(ledger *spendLedger) (bool, error) {
					ledger.clearRetry(entry.id)
					return true, nil
				})
			}
			if err != nil && !errors.Is(err, errBudgetExhausted) {
				if err := s.relayFailed(ctx, entry.id, err); err != nil {
					return err
				}
			}
			if _, err := headers(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}
