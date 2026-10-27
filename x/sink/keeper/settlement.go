package keeper

import (
	"fmt"
	"math"
	"slices"
	"strconv"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/sink/v1"
)

type refundContextKey struct{}
type refundAuthorization struct {
	recipient sdk.AccAddress
	amount    uint64
}

func amountCoins(amount uint64) sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin(types.Denom, sdkmath.NewIntFromUint64(amount)))
}

// settlement loads a transfer and its settlement. Unlike GetTransfer it skips
// the void-order lookup, which settlement code reads only where it needs it.
func (k Keeper) settlement(ctx sdk.Context, id uint64) (types.Transfer, types.Settlement, error) {
	record, ok := get(ctx, k.transfers, id, nil)
	if !ok {
		return types.Transfer{}, types.Settlement{}, sdkerrors.ErrNotFound.Wrap("sink transfer")
	}
	s, _ := get(ctx, k.settlements, id, types.Settlement{Round: 1})
	t, err := types.DecodeTransfer(record)
	return t, s, err
}

func reporterPolicy(p types.Params, epoch uint64) types.ReporterPolicy {
	return types.ReporterPolicy{Epoch: epoch, Members: slices.Clone(p.Reporters), Threshold: p.ReporterThreshold}
}

func challengeDeadline(ctx sdk.Context, p types.Params) (int64, error) {
	now := ctx.BlockTime().Unix()
	if now <= 0 || p.ChallengePeriodSeconds > uint64(math.MaxInt64-now) { //nolint:gosec // now > 0
		return 0, fmt.Errorf("invalid challenge deadline")
	}
	return now + int64(p.ChallengePeriodSeconds), nil //nolint:gosec // bounded by the check above
}

func settlementEvent(ctx sdk.Context, name string, id uint64, attrs ...sdk.Attribute) {
	ctx.EventManager().EmitEvent(sdk.NewEvent(name, append([]sdk.Attribute{sdk.NewAttribute("transfer_id", strconv.FormatUint(id, 10))}, attrs...)...))
}

func bucket(t *types.Totals, s types.Status) *uint64 {
	switch s {
	case types.Status_STATUS_PENDING:
		return &t.PendingAmount
	case types.Status_STATUS_MINTED:
		return &t.MintedAmount
	case types.Status_STATUS_VOIDED:
		return &t.VoidedAmount
	case types.Status_STATUS_REFUNDED:
		return &t.RefundedAmount
	}
	return nil
}

// transition moves one amount between accounting buckets in totals, which the
// caller stores. Terminal outcomes cannot be reopened here, and only their
// first transition frees a pending slot.
func (k Keeper) transition(ctx sdk.Context, totals *types.Totals, t types.Transfer, s *types.Settlement, next types.Status) error {
	if s.Status == next {
		return nil
	}
	if s.Terminal() || (next == types.Status_STATUS_REFUNDED && s.Status != types.Status_STATUS_VOIDED) {
		return fmt.Errorf("cannot change terminal sink outcome")
	}
	from, to := bucket(totals, s.Status), bucket(totals, next)
	if from == nil || to == nil || *from < t.Amount || *to > math.MaxUint64-t.Amount {
		return fmt.Errorf("invalid sink settlement accounting")
	}
	*from -= t.Amount
	*to += t.Amount
	if next == types.Status_STATUS_MINTED || next == types.Status_STATUS_REFUNDED {
		u := k.GetAccountUsage(ctx, t.Sender[:])
		if u.PendingCount == 0 || totals.PendingCount == 0 {
			return fmt.Errorf("invalid sink pending count")
		}
		u.PendingCount--
		totals.PendingCount--
		if err := k.usage.Set(ctx, t.Sender[:], u); err != nil {
			return err
		}
		if err := k.open.Remove(ctx, t.ID); err != nil {
			return err
		}
	}
	s.Status = next
	if next != types.Status_STATUS_VOIDED {
		s.ChallengeEnd = 0
	}
	return nil
}

// reopenMinted moves a Minted transfer under a conflict hold back to Voided.
// A false mint quorum that votes before Solana resolves closes the transfer
// at once; this is governance's way back. The pending slot and open-index
// entry that the mint released come back.
func (k Keeper) reopenMinted(ctx sdk.Context, totals *types.Totals, t types.Transfer, s *types.Settlement) error {
	u := k.GetAccountUsage(ctx, t.Sender[:])
	if totals.MintedAmount < t.Amount || totals.VoidedAmount > math.MaxUint64-t.Amount ||
		totals.PendingCount == math.MaxUint64 || u.PendingCount == math.MaxUint32 {
		return fmt.Errorf("invalid sink settlement accounting")
	}
	totals.MintedAmount -= t.Amount
	totals.VoidedAmount += t.Amount
	totals.PendingCount++
	u.PendingCount++
	if err := k.usage.Set(ctx, t.Sender[:], u); err != nil {
		return err
	}
	if err := k.open.Set(ctx, t.ID); err != nil {
		return err
	}
	s.Status = types.Status_STATUS_VOIDED
	return nil
}

func (k Keeper) ReportOutcome(ctx sdk.Context, msg *types.MsgReportOutcome) error {
	if msg == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("nil report")
	}
	if err := msg.Observation.Validate(); err != nil {
		return err
	}
	p := k.GetParams(ctx)
	if !slices.Contains(p.Reporters, msg.Reporter) {
		return sdkerrors.ErrUnauthorized.Wrap("not a current sink reporter")
	}
	cache, write := ctx.CacheContext()
	totals := k.GetTotals(cache)
	loaded := totals
	epoch := totals.ReporterEpoch
	if msg.ReporterEpoch != epoch {
		return fmt.Errorf("stale reporter epoch")
	}
	t, s, err := k.settlement(cache, msg.Id)
	if err != nil {
		return err
	}
	before := s
	if msg.Round != s.Round {
		return fmt.Errorf("stale report round")
	}
	if s.Policy.Epoch != epoch {
		// Old quorums never carry across rotation. Holds require governance even
		// after rotation, but an unfinished authorization returns to pending.
		if !s.Terminal() {
			if err := k.transition(cache, &totals, t, &s, types.Status_STATUS_PENDING); err != nil {
				return err
			}
			s.GovernanceObservation = nil
		}
		s.Policy = reporterPolicy(p, epoch)
		s.Votes = nil
	}
	for _, vote := range s.Votes {
		if vote.Reporter == msg.Reporter && vote.Observation.Outcome == msg.Observation.Outcome {
			return nil
		}
	}
	s.Votes = append(s.Votes, types.Vote{Reporter: msg.Reporter, Observation: msg.Observation})
	minted, voided := s.Tallies()
	if (minted > 0 && voided > 0) || (s.Status == types.Status_STATUS_MINTED && voided > 0) ||
		((s.Status == types.Status_STATUS_VOIDED || s.Status == types.Status_STATUS_REFUNDED) && minted > 0) {
		s.Hold = types.Hold_HOLD_CONFLICT
		settlementEvent(cache, "sink_conflict", t.ID, sdk.NewAttribute("reporter", msg.Reporter))
	}
	if s.Hold == types.Hold_HOLD_NONE && s.Status == types.Status_STATUS_PENDING {
		if minted >= p.ReporterThreshold {
			if err := k.transition(cache, &totals, t, &s, types.Status_STATUS_MINTED); err != nil {
				return err
			}
			settlementEvent(cache, "sink_mint_confirmed", t.ID)
		} else if voided >= p.ReporterThreshold {
			deadline, err := challengeDeadline(cache, p)
			if err != nil {
				return err
			}
			if err := k.transition(cache, &totals, t, &s, types.Status_STATUS_VOIDED); err != nil {
				return err
			}
			s.ChallengeEnd = deadline
			settlementEvent(cache, "sink_void_confirmed", t.ID, sdk.NewAttribute("challenge_end", strconv.FormatInt(deadline, 10)))
		}
	}
	if totals != loaded {
		if err := k.setTotals(cache, totals); err != nil {
			return err
		}
	}
	if err := k.putSettlement(cache, t.ID, before, s); err != nil {
		return err
	}
	settlementEvent(cache, "sink_report", t.ID, sdk.NewAttribute("reporter", msg.Reporter), sdk.NewAttribute("outcome", msg.Observation.Outcome.String()))
	write()
	return nil
}

func (k Keeper) VetoVoid(ctx sdk.Context, msg *types.MsgVetoVoid) error {
	if msg == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("nil veto")
	}
	if err := types.ValidateReason(msg.Reason); err != nil {
		return err
	}
	gov := msg.Signer == k.authority
	if !gov && !slices.Contains(k.GetParams(ctx).Reporters, msg.Signer) {
		return sdkerrors.ErrUnauthorized.Wrap("veto requires reporter or governance")
	}
	t, s, err := k.settlement(ctx, msg.Id)
	if err != nil {
		return err
	}
	before := s
	if s.Status != types.Status_STATUS_VOIDED {
		return fmt.Errorf("no pending void to veto")
	}
	if !gov && ctx.BlockTime().Unix() >= s.ChallengeEnd {
		return fmt.Errorf("void challenge window ended")
	}
	if s.Hold != types.Hold_HOLD_CONFLICT {
		s.Hold = types.Hold_HOLD_VETO
	}
	s.VetoBy, s.VetoReason = msg.Signer, msg.Reason
	if err := k.putSettlement(ctx, t.ID, before, s); err != nil {
		return err
	}
	settlementEvent(ctx, "sink_veto", t.ID, sdk.NewAttribute("signer", msg.Signer), sdk.NewAttribute("reason", msg.Reason))
	return nil
}

func (k Keeper) ResolveTransfer(ctx sdk.Context, msg *types.MsgResolveTransfer) error {
	if msg == nil || msg.Authority != k.authority {
		return sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}
	if err := types.ValidateReason(msg.Reason); err != nil {
		return err
	}
	var target types.Status
	switch msg.Decision {
	case types.Decision_DECISION_RESTART:
		if msg.Observation != nil {
			return fmt.Errorf("restart must not authorize an observation")
		}
		target = types.Status_STATUS_PENDING
	case types.Decision_DECISION_CONFIRM_MINT, types.Decision_DECISION_CONFIRM_VOID:
		if msg.Observation == nil {
			return fmt.Errorf("governance confirmation requires Solana evidence")
		}
		if err := msg.Observation.Validate(); err != nil {
			return err
		}
		if (msg.Decision == types.Decision_DECISION_CONFIRM_MINT) != (msg.Observation.Outcome == types.Outcome_OUTCOME_MINTED) {
			return fmt.Errorf("decision does not match observation")
		}
		target = types.Status_STATUS_VOIDED
		if msg.Decision == types.Decision_DECISION_CONFIRM_MINT {
			target = types.Status_STATUS_MINTED
		}
	default:
		return fmt.Errorf("invalid governance resolution")
	}
	cache, write := ctx.CacheContext()
	t, s, err := k.settlement(cache, msg.Id)
	if err != nil {
		return err
	}
	before := s
	p := k.GetParams(cache)
	totals := k.GetTotals(cache)
	loaded := totals
	reopen := false
	if s.Terminal() {
		if s.Status == types.Status_STATUS_REFUNDED && target == types.Status_STATUS_VOIDED {
			target = types.Status_STATUS_REFUNDED
		}
		// Only a Minted transfer that an honest reporter contested can return.
		reopen = s.Status == types.Status_STATUS_MINTED && s.Hold == types.Hold_HOLD_CONFLICT && target == types.Status_STATUS_VOIDED
		if target != s.Status && !reopen {
			return fmt.Errorf("cannot change terminal sink outcome")
		}
	}
	// Akash can check only the observation's format. A void order lets anyone
	// seal the record on Solana with void_transfer during the challenge window,
	// so a governance-confirmed void cannot also be minted. Put MsgRequestVoid
	// in the same proposal.
	if target == types.Status_STATUS_VOIDED && !has(cache, k.voids, msg.Id) {
		return fmt.Errorf("confirming a void requires a void order; include MsgRequestVoid in the proposal")
	}
	if s.Round == math.MaxUint64 {
		return fmt.Errorf("report round exhausted")
	}
	if reopen {
		// The refund queue pays only its head, so it must fit a day's cap.
		if t.Amount > p.DailyRefundCap {
			return fmt.Errorf("transfer amount %d exceeds the daily refund cap; raise the cap first", t.Amount)
		}
		if err := k.reopenMinted(cache, &totals, t, &s); err != nil {
			return err
		}
	}
	if err := k.transition(cache, &totals, t, &s, target); err != nil {
		return err
	}
	s.Round++
	s.Policy = reporterPolicy(p, totals.ReporterEpoch)
	s.Votes = nil
	s.Hold, s.VetoBy, s.VetoReason = types.Hold_HOLD_NONE, "", ""
	s.GovernanceObservation = msg.Observation
	if target == types.Status_STATUS_VOIDED {
		s.ChallengeEnd, err = challengeDeadline(cache, p)
		if err != nil {
			return err
		}
	}
	if totals != loaded {
		if err := k.setTotals(cache, totals); err != nil {
			return err
		}
	}
	if err := k.putSettlement(cache, t.ID, before, s); err != nil {
		return err
	}
	settlementEvent(cache, "sink_resolved", t.ID, sdk.NewAttribute("decision", msg.Decision.String()), sdk.NewAttribute("reason", msg.Reason))
	write()
	return nil
}

// FinalizeRefund is the only operation that can authorize an outgoing bank
// send. It pays exactly one original sender and never releases minted escrow.
func (k Keeper) FinalizeRefund(ctx sdk.Context, id uint64) error {
	cache, write := ctx.CacheContext()
	t, s, err := k.settlement(cache, id)
	if err != nil {
		return err
	}
	if s.Status == types.Status_STATUS_REFUNDED {
		return nil
	}
	before := s
	p := k.GetParams(cache)
	totals := k.GetTotals(cache)
	if !p.RefundsEnabled {
		return fmt.Errorf("sink refunds disabled")
	}
	if s.Status != types.Status_STATUS_VOIDED || !s.HasVoidAuthorization() || s.Hold != types.Hold_HOLD_NONE {
		return fmt.Errorf("refund requires an unchallenged confirmed void")
	}
	if s.Policy.Epoch != totals.ReporterEpoch {
		return fmt.Errorf("stale void authorization after reporter rotation")
	}
	now := cache.BlockTime().Unix()
	if s.ChallengeEnd <= 0 || now < s.ChallengeEnd {
		return fmt.Errorf("void challenge window not finished")
	}
	// Oldest first: a refund waits only for refunds that matured before it.
	head, ok, err := k.refundQueueHead(cache)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("sink refund queue is missing transfer %d", id)
	}
	if head != id {
		return fmt.Errorf("sink refund %d must wait for older refund %d", id, head)
	}
	day := uint64(now / 86400) //nolint:gosec // now >= ChallengeEnd > 0
	if day < totals.RefundDay {
		return fmt.Errorf("sink clock moved backwards")
	}
	if day != totals.RefundDay {
		totals.RefundDay, totals.RefundedToday = day, 0
	}
	if totals.RefundedToday > p.DailyRefundCap || t.Amount > p.DailyRefundCap-totals.RefundedToday {
		return fmt.Errorf("sink daily refund cap")
	}
	if err := k.CheckBalance(cache); err != nil {
		return err
	}
	// Record the refund before paying it.
	totals.RefundedToday += t.Amount
	if err := k.transition(cache, &totals, t, &s, types.Status_STATUS_REFUNDED); err != nil {
		return err
	}
	if err := k.setTotals(cache, totals); err != nil {
		return err
	}
	if err := k.putSettlement(cache, t.ID, before, s); err != nil {
		return err
	}
	recipient := sdk.AccAddress(t.Sender[:])
	balance := k.bank.GetAllBalances(cache, recipient)
	send := cache.WithValue(refundContextKey{}, refundAuthorization{recipient: recipient, amount: t.Amount})
	if err := k.bank.SendCoinsFromModuleToAccount(send, types.ModuleName, recipient, amountCoins(t.Amount)); err != nil {
		return err
	}
	// A send restriction added after the sink's could redirect the payment;
	// the sink side would look the same either way.
	if !k.bank.GetAllBalances(cache, recipient).Equal(balance.Add(amountCoins(t.Amount)...)) {
		return fmt.Errorf("sink refund did not reach the original sender")
	}
	if err := k.CheckBalance(cache); err != nil {
		return err
	}
	settlementEvent(cache, "sink_refunded", t.ID, sdk.NewAttribute("sender", recipient.String()), sdk.NewAttribute("amount", strconv.FormatUint(t.Amount, 10)))
	write()
	return nil
}
