package keeper

import (
	"math"
	"slices"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	gogotypes "github.com/cosmos/gogoproto/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

func challengeDeadline(ctx sdk.Context, p types.Params) (*time.Time, error) {
	deadline := ctx.BlockTime().Add(p.ReporterChallengePeriod).UTC()
	if !deadline.After(ctx.BlockTime()) {
		return nil, types.ErrInvalidClock.Wrap("invalid challenge deadline")
	}

	if _, err := gogotypes.TimestampProto(deadline); err != nil {
		return nil, err
	}

	return &deadline, nil
}

// RequestVoid commits a governance order for proof on Solana. It does not
// establish that Solana executed the void and cannot release any escrow.
func (k Keeper) RequestVoid(ctx sdk.Context, msg *types.MsgRequestVoid) error {
	if msg == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("nil request")
	}

	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	cache, write := ctx.CacheContext()

	if msg.Authority != k.authority {
		return sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}

	t, err := k.GetTransfer(cache, msg.ID)
	if err != nil {
		return err
	}

	if t.VoidOrdered {
		return nil
	}

	s := t.Settlement
	if s.Status == types.StatusRefunded || s.Status == types.StatusMinted && s.Hold != types.HoldConflict {
		return types.ErrTransferClosed
	}

	if err = k.voids.Set(cache, msg.ID, types.VoidOrder{}); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventVoidOrdered{ID: msg.ID}); err != nil {
		return err
	}

	write()

	return nil
}

func (k Keeper) ReportOutcome(ctx sdk.Context, msg *types.MsgReportOutcome) error {
	if msg == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("nil report")
	}

	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	cache, write := ctx.CacheContext()
	policy, err := k.currentPolicy(cache)
	if err != nil {
		return err
	}

	if !slices.Contains(policy.Members, msg.Reporter) {
		return types.ErrNotReporter
	}

	if msg.PolicyVersion != policy.Version {
		return types.ErrStaleReport
	}

	t, err := k.GetTransfer(cache, msg.ID)
	if err != nil {
		return err
	}

	before := t.Settlement
	s := before
	if msg.Round != s.Round {
		return types.ErrStaleReport
	}

	refreshed := false
	if s.PolicyVersion != policy.Version && (s.Status == types.StatusTransferPending || s.Status == types.StatusRefundPending && s.GovernanceEvidence == nil) {
		refreshed = true
		s.Status = types.StatusTransferPending
		s.PolicyVersion = policy.Version
		s.ChallengeEnd = nil
	}

	duplicate := false
	for _, v := range s.Votes {
		if v.Reporter == msg.Reporter && v.Observation.Outcome == msg.Observation.Outcome && v.PolicyVersion == msg.PolicyVersion {
			duplicate = true
			break
		}
	}

	if duplicate && !refreshed {
		return nil
	}

	conflict := s.Status == types.StatusMinted && msg.Observation.Outcome == types.OutcomeVoided || (s.Status == types.StatusRefundPending || s.Status == types.StatusRefunded) && msg.Observation.Outcome == types.OutcomeMinted
	for _, v := range s.Votes {
		if v.Observation.Outcome != msg.Observation.Outcome {
			conflict = true
		}
	}

	if !duplicate {
		s.Votes = append(s.Votes, types.Vote{Reporter: msg.Reporter, Observation: msg.Observation, PolicyVersion: msg.PolicyVersion})
	}

	if conflict && s.Hold != types.HoldConflict {
		s.Hold = types.HoldConflict

		if err = cache.EventManager().EmitTypedEvent(&types.EventReportConflict{ID: msg.ID, Reporter: msg.Reporter}); err != nil {
			return err
		}
	}

	if s.Hold == types.HoldNone && s.Status == types.StatusTransferPending {
		minted, voided := s.Tallies()
		if policy.Threshold > 0 && minted >= policy.Threshold {
			s.Status = types.StatusMinted

			if err = cache.EventManager().EmitTypedEvent(&types.EventMintConfirmed{ID: msg.ID}); err != nil {
				return err
			}
		} else if policy.Threshold > 0 && voided >= policy.Threshold {
			p, err := k.params.Get(cache)
			if err != nil {
				return err
			}

			s.Status = types.StatusRefundPending
			s.ChallengeEnd, err = challengeDeadline(cache, p)
			if err != nil {
				return err
			}

			if err = cache.EventManager().EmitTypedEvent(&types.EventVoidConfirmed{ID: msg.ID, ChallengeEnd: *s.ChallengeEnd}); err != nil {
				return err
			}
		}
	}

	if err = k.put(cache, msg.ID, t.Data.Amount, &before, s); err != nil {
		return err
	}

	if !duplicate {
		if err = cache.EventManager().EmitTypedEvent(&types.EventOutcomeReported{
			ID:       msg.ID,
			Reporter: msg.Reporter,
			Outcome:  msg.Observation.Outcome,
		}); err != nil {
			return err
		}
	}

	write()

	return nil
}

func (k Keeper) VetoVoid(ctx sdk.Context, msg *types.MsgVetoVoid) error {
	if msg == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("nil veto")
	}

	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	cache, write := ctx.CacheContext()
	policy, err := k.currentPolicy(cache)
	if err != nil {
		return err
	}

	gov := msg.Signer == k.authority
	if !gov && !slices.Contains(policy.Members, msg.Signer) {
		return types.ErrNotReporter
	}

	t, err := k.GetTransfer(cache, msg.ID)
	if err != nil {
		return err
	}

	before := t.Settlement
	s := before
	if s.Status != types.StatusRefundPending {
		return types.ErrNoPendingVoid
	}

	if s.ChallengeEnd == nil {
		return types.ErrInvalidSettlement
	}

	if !gov && !cache.BlockTime().Before(*s.ChallengeEnd) {
		return types.ErrChallengeWindowEnded
	}

	if s.Hold != types.HoldConflict {
		s.Hold = types.HoldVeto
	}

	s.Veto = &types.Veto{Signer: msg.Signer, Reason: msg.Reason}
	if err = k.put(cache, msg.ID, t.Data.Amount, &before, s); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventVoidVetoed{ID: msg.ID, Signer: msg.Signer, Reason: msg.Reason}); err != nil {
		return err
	}

	write()

	return nil
}

func (k Keeper) Resolve(ctx sdk.Context, msg types.Resolution) error {
	var authority, reason string
	var id types.ID
	var round uint64
	var decision types.Decision
	var evidence *types.SolanaEvidence
	switch m := msg.(type) {
	case *types.MsgRestartTransfer:
		if m == nil {
			return sdkerrors.ErrInvalidRequest.Wrap("nil restart")
		}

		authority, reason, id, round, decision = m.Authority, m.Reason, m.ID, m.ExpectedRound, types.DecisionRestart
	case *types.MsgConfirmMint:
		if m == nil {
			return sdkerrors.ErrInvalidRequest.Wrap("nil confirmation")
		}

		authority, reason, id, round, decision = m.Authority, m.Reason, m.ID, m.ExpectedRound, types.DecisionConfirmMint
		evidence = &m.Evidence
	case *types.MsgConfirmVoid:
		if m == nil {
			return sdkerrors.ErrInvalidRequest.Wrap("nil confirmation")
		}

		authority, reason, id, round, decision = m.Authority, m.Reason, m.ID, m.ExpectedRound, types.DecisionConfirmVoid
		evidence = &m.Evidence
	default:
		return types.ErrInvalidResolution
	}

	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	cache, write := ctx.CacheContext()

	if authority != k.authority {
		return sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}

	t, err := k.GetTransfer(cache, id)
	if err != nil {
		return err
	}

	before := t.Settlement
	s := before
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
		if s.Terminal() {
			return types.ErrTransferClosed
		}

		s.Status = types.StatusTransferPending
	case types.DecisionConfirmMint:
		if s.Status == types.StatusRefunded {
			return types.ErrTransferClosed
		}

		s.Status = types.StatusMinted
	case types.DecisionConfirmVoid:
		if s.Status != types.StatusRefunded {
			if s.Status == types.StatusMinted && s.Hold != types.HoldConflict {
				return types.ErrTransferClosed
			}

			if !t.VoidOrdered {
				return types.ErrInvalidResolution.Wrap("confirming an unpaid void requires a void order")
			}

			if t.Data.Amount.IsGT(p.MaxEpochRefund) {
				return types.ErrRefundCapReached
			}

			s.Status = types.StatusRefundPending
		}
	}

	policy, err := k.currentPolicy(cache)
	if err != nil {
		return err
	}

	s.Round++
	s.PolicyVersion = policy.Version
	s.Votes = nil
	s.Hold = types.HoldNone
	s.Veto = nil
	s.ChallengeEnd = nil
	s.GovernanceEvidence = evidence
	if s.Status == types.StatusRefundPending {
		s.ChallengeEnd, err = challengeDeadline(cache, p)
		if err != nil {
			return err
		}
	}

	if err = k.put(cache, id, t.Data.Amount, &before, s); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventTransferResolved{ID: id, Decision: decision, Reason: reason}); err != nil {
		return err
	}

	write()

	return nil
}

// FinalizeRefund pays exactly one original sender and never releases minted
// escrow.
func (k Keeper) FinalizeRefund(ctx sdk.Context, msg *types.MsgFinalizeRefund) error {
	if msg == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("nil refund")
	}

	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	cache, write := ctx.CacheContext()
	t, err := k.GetTransfer(cache, msg.ID)
	if err != nil {
		return err
	}

	before := t.Settlement
	s := before
	if s.Status == types.StatusRefunded {
		return nil
	}

	p, err := k.params.Get(cache)
	if err != nil {
		return err
	}

	if !p.RefundsEnabled {
		return types.ErrRefundsDisabled
	}

	policy, err := k.currentPolicy(cache)
	if err != nil {
		return err
	}

	if !s.Payable(policy, policy.Version) {
		return types.ErrInvalidSettlement.Wrap("refund needs current authorization without a hold")
	}

	if s.ChallengeEnd == nil || cache.BlockTime().Before(*s.ChallengeEnd) {
		return types.ErrChallengeWindowNotElapsed
	}

	head, ok, err := k.refundHead(cache, policy.Version)
	if err != nil {
		return err
	}

	if !ok || head != msg.ID {
		return types.ErrRefundOutOfOrder
	}

	epoch, err := types.EpochOf(cache.BlockTime())
	if err != nil {
		return err
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

	s.Status = types.StatusRefunded
	s.ChallengeEnd = nil
	if err = k.put(cache, msg.ID, t.Data.Amount, &before, s); err != nil {
		return err
	}

	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		return err
	}

	balance := k.bank.GetAllBalances(cache, owner)
	coins := sdk.NewCoins(t.Data.Amount)
	send := cache.WithValue(refundContextKey{}, refundAuthorization{recipient: owner, amount: coins})
	if err = k.bank.SendCoinsFromModuleToAccount(send, types.ModuleName, owner, coins); err != nil {
		return err
	}

	if !k.bank.GetAllBalances(cache, owner).Equal(balance.Add(coins...)) {
		return types.ErrBalanceMismatch.Wrap("refund did not reach original sender")
	}

	if err = k.CheckBalance(cache); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventTransferRefunded{ID: msg.ID, Sender: msg.ID.Owner, Amount: t.Data.Amount}); err != nil {
		return err
	}

	write()

	return nil
}
