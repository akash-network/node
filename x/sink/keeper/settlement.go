package keeper

import (
	"math"
	"slices"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	gogotypes "github.com/cosmos/gogoproto/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

func challengeDeadline(ctx sdk.Context, p types.Params) (int64, error) {
	now := ctx.BlockTime()
	duration := p.ReporterChallengePeriod
	if duration <= 0 || now.Unix() < 0 {
		return 0, types.ErrInvalidClock.Wrap("invalid challenge clock")
	}

	seconds := int64(duration / time.Second)
	nanos := int64(now.Nanosecond()) + int64(duration%time.Second)
	seconds += (nanos + int64(time.Second) - 1) / int64(time.Second)
	if now.Unix() > math.MaxInt64-seconds {
		return 0, types.ErrInvalidClock.Wrap("challenge deadline overflow")
	}

	deadline := now.Unix() + seconds
	if deadline <= 0 {
		return 0, types.ErrInvalidClock.Wrap("invalid challenge deadline")
	}

	if _, err := gogotypes.TimestampProto(time.Unix(deadline, 0)); err != nil {
		return 0, types.ErrInvalidClock.Wrap(err.Error())
	}

	return deadline, nil
}

// RequestVoid commits a permanent governance order for proof on Solana.
// The order cannot release escrow without a separate final void authorization.
func (k *keeper) RequestVoid(ctx sdk.Context, msg *types.MsgRequestVoid) error {
	cache, write := ctx.CacheContext()
	t, err := k.GetTransfer(cache, msg.ID)
	if err != nil {
		return err
	}

	if t.Settlement.VoidAuthorization != nil {
		return nil
	}

	if msg.ExpectedRound != t.Settlement.Round {
		return types.ErrStaleReport.Wrap("expected_round differs from settlement")
	}

	if t.Status == types.StatusRefunded || t.Status == types.StatusMinted && t.Settlement.Hold != types.HoldConflict {
		return types.ErrTransferClosed
	}

	if cache.BlockHeight() < t.ID.Height {
		return types.ErrInvalidClock.Wrap("void request precedes deposit")
	}

	before := t
	t.Settlement.VoidAuthorization = &types.VoidAuthorization{
		RequestedHeight: cache.BlockHeight(),
		RequestedRound:  t.Settlement.Round,
	}

	if t.Status == types.StatusTransferPending {
		t.Status = types.StatusVoided
	}

	if err = k.put(cache, &before, t); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventVoidOrdered{ID: msg.ID}); err != nil {
		return err
	}

	write()

	return nil
}

func (k *keeper) ReportOutcome(ctx sdk.Context, msg *types.MsgReportOutcome) error {
	cache, write := ctx.CacheContext()
	policy, err := k.GetCurrentPolicy(cache)
	if err != nil {
		return err
	}

	if msg.PolicyVersion != policy.Version {
		return types.ErrStaleReport
	}

	t, err := k.GetTransfer(cache, msg.ID)
	if err != nil {
		return err
	}

	before := t
	s := &t.Settlement
	if msg.Round != s.Round {
		return types.ErrStaleReport
	}

	if s.Hold == types.HoldConflict {
		return nil
	}

	if s.Policy.Version != policy.Version && s.Hold == types.HoldNone && !t.Status.Terminal() && s.GovernanceObservation == nil {
		s.Policy, s.Votes, s.ChallengeEnd = policy, nil, 0
		t.Status = types.StatusTransferPending
		if s.VoidAuthorization != nil {
			t.Status = types.StatusVoided
		}
	}

	primary := types.OutcomeUnspecified
	if s.GovernanceObservation != nil {
		primary = s.GovernanceObservation.Outcome
	} else if len(s.Votes) > 0 {
		primary = s.Votes[0].Observation.Outcome
	}

	conflict := primary != types.OutcomeUnspecified && primary != msg.Observation.Outcome
	if !conflict {
		if t.Status.Terminal() || t.Status == types.StatusRefundPending || s.Hold != types.HoldNone {
			return nil
		}

		for _, vote := range s.Votes {
			if vote.Reporter == msg.Reporter {
				return nil
			}
		}
	}

	s.Votes = append(slices.Clone(s.Votes), types.Vote{
		Reporter:      msg.Reporter,
		Observation:   msg.Observation,
		PolicyVersion: msg.PolicyVersion,
	})

	if conflict {
		s.Hold = types.HoldConflict

		if err = cache.EventManager().EmitTypedEvent(&types.EventReportConflict{
			ID:       msg.ID,
			Reporter: msg.Reporter,
		}); err != nil {
			return err
		}
	} else {
		minted, voided := s.Tallies()
		if policy.Threshold > 0 && minted == policy.Threshold {
			t.Status = types.StatusMinted

			if err = cache.EventManager().EmitTypedEvent(&types.EventMintConfirmed{ID: msg.ID}); err != nil {
				return err
			}
		} else if policy.Threshold > 0 && voided == policy.Threshold {
			p, err := k.params.Get(cache)
			if err != nil {
				return err
			}

			t.Status = types.StatusRefundPending
			s.ChallengeEnd, err = challengeDeadline(cache, p)
			if err != nil {
				return err
			}

			if err = cache.EventManager().EmitTypedEvent(&types.EventVoidConfirmed{
				ID:           msg.ID,
				ChallengeEnd: time.Unix(s.ChallengeEnd, 0).UTC(),
			}); err != nil {
				return err
			}
		}
	}

	if err = k.put(cache, &before, t); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventOutcomeReported{
		ID:       msg.ID,
		Reporter: msg.Reporter,
		Outcome:  msg.Observation.Outcome,
	}); err != nil {
		return err
	}

	write()

	return nil
}

func (k *keeper) VetoVoid(ctx sdk.Context, msg *types.MsgVetoVoid) error {
	cache, write := ctx.CacheContext()
	policy, err := k.GetCurrentPolicy(cache)
	if err != nil {
		return err
	}

	gov := msg.Signer == k.authority
	t, err := k.GetTransfer(cache, msg.ID)
	if err != nil {
		return err
	}

	before := t
	s := &t.Settlement

	if t.Status != types.StatusRefundPending {
		return types.ErrNoPendingVoid
	}

	if s.ChallengeEnd == 0 {
		return types.ErrInvalidSettlement
	}

	if !gov && cache.BlockTime().Unix() >= s.ChallengeEnd {
		return types.ErrChallengeWindowEnded
	}

	if s.Hold != types.HoldConflict {
		s.Hold = types.HoldVeto
	}

	s.VetoBy, s.VetoReason = msg.Signer, msg.Reason
	s.VetoPolicyVersion = policy.Version
	if gov {
		s.VetoPolicyVersion = types.GovernanceRefundScope
	}

	if err = k.put(cache, &before, t); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventVoidVetoed{
		ID:     msg.ID,
		Signer: msg.Signer,
		Reason: msg.Reason,
	}); err != nil {
		return err
	}

	write()

	return nil
}

func (k *keeper) Resolve(ctx sdk.Context, msg types.Resolution) error {
	var reason string
	var id types.ID
	var round uint64
	var decision types.Decision
	var evidence *types.SolanaEvidence
	switch m := msg.(type) {
	case *types.MsgRestartTransfer:
		reason, id, round, decision = m.Reason, m.ID, m.ExpectedRound, types.DecisionRestart
	case *types.MsgConfirmMint:
		reason, id, round, decision = m.Reason, m.ID, m.ExpectedRound, types.DecisionConfirmMint
		evidence = &m.Evidence
	case *types.MsgConfirmVoid:
		reason, id, round, decision = m.Reason, m.ID, m.ExpectedRound, types.DecisionConfirmVoid
		evidence = &m.Evidence
	default:
		return types.ErrInvalidResolution
	}

	cache, write := ctx.CacheContext()

	t, err := k.GetTransfer(cache, id)
	if err != nil {
		return err
	}

	before := t
	s := &t.Settlement
	if s.Round != round {
		return types.ErrStaleReport.Wrap("expected_round differs from settlement")
	}

	if s.Round == math.MaxUint64 {
		return types.ErrOverflow.Wrap("settlement round")
	}

	p, err := k.params.Get(cache)
	if err != nil {
		return err
	}

	switch decision {
	case types.DecisionRestart:
		if t.Status.Terminal() {
			return types.ErrTransferClosed
		}

		t.Status = types.StatusTransferPending
		if s.VoidAuthorization != nil {
			t.Status = types.StatusVoided
		}
	case types.DecisionConfirmMint:
		if t.Status == types.StatusRefunded {
			return types.ErrTransferClosed
		}

		t.Status = types.StatusMinted
	case types.DecisionConfirmVoid:
		if t.Status != types.StatusRefunded {
			if t.Status == types.StatusMinted && s.Hold != types.HoldConflict {
				return types.ErrTransferClosed
			}

			if s.VoidAuthorization == nil {
				return types.ErrInvalidResolution.Wrap("confirming an unpaid void requires a void order")
			}

			if t.Data.Amount.IsGT(p.MaxEpochRefund) {
				return types.ErrRefundCapReached
			}

			t.Status = types.StatusRefundPending
		}
	}

	policy, err := k.GetCurrentPolicy(cache)
	if err != nil {
		return err
	}

	s.Round++
	s.Policy = policy
	s.Votes = nil
	s.Hold = types.HoldNone
	s.VetoBy, s.VetoReason, s.VetoPolicyVersion = "", "", 0
	s.ChallengeEnd = 0
	s.GovernanceObservation = nil
	if evidence != nil {
		outcome := types.OutcomeMinted
		if decision == types.DecisionConfirmVoid {
			outcome = types.OutcomeVoided
		}

		s.GovernanceObservation = &types.Observation{
			Outcome:              outcome,
			Slot:                 evidence.Slot,
			TransactionSignature: evidence.TransactionSignature,
		}
	}

	if t.Status == types.StatusRefundPending {
		s.ChallengeEnd, err = challengeDeadline(cache, p)
		if err != nil {
			return err
		}
	}

	if err = k.put(cache, &before, t); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventTransferResolved{
		ID:       id,
		Decision: decision,
		Reason:   reason,
	}); err != nil {
		return err
	}

	write()

	return nil
}

// FinalizeRefund pays exactly one original sender and never releases minted
// escrow.
func (k *keeper) FinalizeRefund(ctx sdk.Context, msg *types.MsgFinalizeRefund) error {
	cache, write := ctx.CacheContext()
	t, err := k.GetTransfer(cache, msg.ID)
	if err != nil {
		return err
	}

	before := t
	s := &t.Settlement

	if t.Status == types.StatusRefunded {
		return nil
	}

	p, err := k.params.Get(cache)
	if err != nil {
		return err
	}

	if !p.RefundsEnabled {
		return types.ErrRefundsDisabled
	}

	policy, err := k.GetCurrentPolicy(cache)
	if err != nil {
		return err
	}

	if !s.Payable(t.Status, policy.Version) {
		if t.Status == types.StatusRefundPending && s.HasVoidAuthorization() {
			switch s.Hold {
			case types.HoldVeto, types.HoldConflict:
				return types.ErrSettlementHeld
			case types.HoldNone:
				return types.ErrStaleReport
			}
		}

		return types.ErrInvalidSettlement.Wrap("refund needs current authorization without a hold")
	}

	if s.ChallengeEnd == 0 || cache.BlockTime().Unix() < s.ChallengeEnd {
		return types.ErrChallengeWindowNotElapsed
	}

	head, ok, err := k.refundHead(cache, policy.Version)
	if err != nil {
		return err
	}

	if !ok || head != msg.ID {
		return types.ErrRefundOutOfOrder
	}

	epoch, err := k.quotaEpoch(cache)
	if err != nil {
		return err
	}

	if epoch == 0 {
		return types.ErrInvalidClock.Wrap("quota epoch has not started")
	}

	usage, err := k.refundUsage.Get(cache)
	if err != nil {
		return err
	}

	usage, err = usage.At(epoch)
	if err != nil {
		return err
	}

	usage.Amount, err = types.AddAmount(usage.Amount, t.Data.Amount)
	if err != nil {
		return err
	}

	if usage.Amount.IsGT(p.MaxEpochRefund) {
		return types.ErrRefundCapReached
	}

	if err = k.CheckBalance(cache); err != nil {
		return err
	}

	if err = k.refundUsage.Set(cache, usage); err != nil {
		return err
	}

	t.Status = types.StatusRefunded
	s.ChallengeEnd = 0

	if err = k.put(cache, &before, t); err != nil {
		return err
	}

	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		return err
	}

	balance := k.bank.GetAllBalances(cache, owner)
	coins := sdk.NewCoins(t.Data.Amount)
	if err = k.bank.SendCoinsFromModuleToAccount(cache, types.ModuleName, owner, coins); err != nil {
		return err
	}

	if !k.bank.GetAllBalances(cache, owner).Equal(balance.Add(coins...)) {
		return types.ErrBalanceMismatch.Wrap("refund did not reach original sender")
	}

	if err = k.CheckBalance(cache); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventTransferRefunded{
		ID:     msg.ID,
		Sender: msg.ID.Owner,
		Amount: t.Data.Amount,
	}); err != nil {
		return err
	}

	write()

	return nil
}
