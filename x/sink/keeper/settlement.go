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

func (k Keeper) setSettlement(ctx sdk.Context, id uint64, s types.Settlement) {
	ctx.KVStore(k.key).Set(types.SettlementKey(id), k.cdc.MustMarshal(&s))
}

func (k Keeper) settlement(ctx sdk.Context, id uint64) (types.Transfer, types.Settlement, error) {
	entry, ok := k.GetTransfer(ctx, id)
	if !ok {
		return types.Transfer{}, types.Settlement{}, sdkerrors.ErrNotFound.Wrap("sink transfer")
	}
	t, err := types.DecodeTransfer(entry.Record)
	return t, entry.Settlement, err
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

// transition moves one amount between accounting buckets. Terminal outcomes
// cannot be reopened, and only their first transition frees a pending slot.
func (k Keeper) transition(ctx sdk.Context, t types.Transfer, s *types.Settlement, next types.Status) error {
	if s.Status == next {
		return nil
	}
	if s.Terminal() || (next == types.Status_STATUS_REFUNDED && s.Status != types.Status_STATUS_VOIDED) {
		return fmt.Errorf("cannot change terminal sink outcome")
	}
	totals := k.GetTotals(ctx)
	buckets := map[types.Status]*uint64{
		types.Status_STATUS_PENDING: &totals.PendingAmount, types.Status_STATUS_MINTED: &totals.MintedAmount,
		types.Status_STATUS_VOIDED: &totals.VoidedAmount, types.Status_STATUS_REFUNDED: &totals.RefundedAmount,
	}
	from, to := buckets[s.Status], buckets[next]
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
		ctx.KVStore(k.key).Set(accountKey(t.Sender[:]), k.cdc.MustMarshal(&u))
		ctx.KVStore(k.key).Delete(openKey(t.ID))
	}
	k.setTotals(ctx, totals)
	s.Status = next
	if next != types.Status_STATUS_VOIDED {
		s.ChallengeEnd = 0
	}
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
	epoch := k.GetTotals(ctx).ReporterEpoch
	if !slices.Contains(p.Reporters, msg.Reporter) {
		return sdkerrors.ErrUnauthorized.Wrap("not a current sink reporter")
	}
	if msg.ReporterEpoch != epoch {
		return fmt.Errorf("stale reporter epoch")
	}
	cache, write := ctx.CacheContext()
	t, s, err := k.settlement(cache, msg.Id)
	if err != nil {
		return err
	}
	if msg.Round != s.Round {
		return fmt.Errorf("stale report round")
	}
	if s.Policy.Epoch != epoch {
		// Old quorums never carry across rotation. Holds require governance even
		// after rotation, but an unfinished authorization returns to pending.
		if !s.Terminal() {
			if err := k.transition(cache, t, &s, types.Status_STATUS_PENDING); err != nil {
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
			if err := k.transition(cache, t, &s, types.Status_STATUS_MINTED); err != nil {
				return err
			}
			settlementEvent(cache, "sink_mint_confirmed", t.ID)
		} else if voided >= p.ReporterThreshold {
			deadline, err := challengeDeadline(cache, p)
			if err != nil {
				return err
			}
			if err := k.transition(cache, t, &s, types.Status_STATUS_VOIDED); err != nil {
				return err
			}
			s.ChallengeEnd = deadline
			settlementEvent(cache, "sink_void_confirmed", t.ID, sdk.NewAttribute("challenge_end", strconv.FormatInt(deadline, 10)))
		}
	}
	k.setSettlement(cache, t.ID, s)
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
	k.setSettlement(ctx, t.ID, s)
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
	if s.Terminal() {
		if s.Status == types.Status_STATUS_REFUNDED && target == types.Status_STATUS_VOIDED {
			target = types.Status_STATUS_REFUNDED
		}
		if target != s.Status {
			return fmt.Errorf("cannot change terminal sink outcome")
		}
	}
	if s.Round == math.MaxUint64 {
		return fmt.Errorf("report round exhausted")
	}
	if err := k.transition(cache, t, &s, target); err != nil {
		return err
	}
	s.Round++
	s.Policy = reporterPolicy(k.GetParams(cache), k.GetTotals(cache).ReporterEpoch)
	s.Votes = nil
	s.Hold, s.VetoBy, s.VetoReason = types.Hold_HOLD_NONE, "", ""
	s.GovernanceObservation = msg.Observation
	if target == types.Status_STATUS_VOIDED {
		s.ChallengeEnd, err = challengeDeadline(cache, k.GetParams(cache))
		if err != nil {
			return err
		}
	}
	k.setSettlement(cache, t.ID, s)
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
	cache = cache.WithValue(refundContextKey{}, refundAuthorization{recipient: t.Sender[:], amount: t.Amount})
	if err := k.bank.SendCoinsFromModuleToAccount(cache, types.ModuleName, t.Sender[:], amountCoins(t.Amount)); err != nil {
		return err
	}
	totals.RefundedToday += t.Amount
	k.setTotals(cache, totals)
	if err := k.transition(cache, t, &s, types.Status_STATUS_REFUNDED); err != nil {
		return err
	}
	k.setSettlement(cache, t.ID, s)
	if err := k.CheckBalance(cache); err != nil {
		return err
	}
	settlementEvent(cache, "sink_refunded", t.ID, sdk.NewAttribute("sender", sdk.AccAddress(t.Sender[:]).String()), sdk.NewAttribute("amount", strconv.FormatUint(t.Amount, 10)))
	write()
	return nil
}
