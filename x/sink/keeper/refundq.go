package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

func refundKey(t types.TransferState) (collections.Triple[uint64, int64, types.ID], bool) {
	scope, ok := t.Settlement.RefundScope(t.Status)
	if !ok || t.Settlement.ChallengeEnd <= 0 {
		return collections.Triple[uint64, int64, types.ID]{}, false
	}

	return collections.Join3(scope, t.Settlement.ChallengeEnd, t.ID), true
}

func (k *keeper) validateSettlement(ctx context.Context, t types.TransferState) error {
	policies, err := k.referencedPolicies(ctx, t.Settlement)
	if err != nil {
		return err
	}

	if err = t.Settlement.Validate(t.ID, t.Status, policies); err != nil {
		return err
	}

	s := t.Settlement
	if s.VetoBy != "" && s.VetoPolicyVersion == types.GovernanceRefundScope && s.VetoBy != k.authority {
		return types.ErrInvalidSettlement.Wrap("governance veto signer differs from authority")
	}

	return nil
}

func (k *keeper) put(ctx sdk.Context, before *types.TransferState, after types.TransferState) error {
	if err := k.validateSettlement(ctx, after); err != nil {
		return err
	}

	if before != nil {
		if before.ID != after.ID || before.Data.Dest != after.Data.Dest || !before.Data.Amount.Equal(after.Data.Amount) {
			return types.ErrInternal.Wrap("immutable transfer changed")
		}

		if receipt := before.Settlement.VoidAuthorization; receipt != nil {
			next := after.Settlement.VoidAuthorization
			if next == nil || *receipt != *next {
				return types.ErrInvalidSettlement.Wrap("permanent void authorization changed")
			}
		}
	} else {
		for status := types.StatusTransferPending; status <= types.StatusRefunded; status++ {
			exists, err := k.settlements.Has(ctx, types.NewSettlementID(status, after.ID))
			if err != nil {
				return err
			}

			if exists {
				return types.ErrInternal.Wrap("canonical settlement already exists")
			}
		}
	}

	if before == nil || before.Status != after.Status {
		totals, err := k.totals.Get(ctx)
		if err != nil {
			return err
		}

		if err = totals.Validate(); err != nil {
			return err
		}

		if before != nil {
			if err = totals.Sub(before.Status, after.Data.Amount); err != nil {
				return types.ErrInternal.Wrap("invalid settlement accounting")
			}

			if err = k.settlements.Remove(ctx, types.NewSettlementID(before.Status, after.ID)); err != nil {
				return err
			}
		}

		if err = totals.Add(after.Status, after.Data.Amount); err != nil {
			return err
		}

		if err = k.totals.Set(ctx, totals); err != nil {
			return err
		}

		epoch, err := k.quotaEpoch(ctx)
		if err != nil {
			return err
		}

		usage, err := k.refundUsage.Get(ctx)
		if err != nil {
			return err
		}

		usage, err = usage.At(epoch)
		if err != nil {
			return err
		}

		if err = ctx.EventManager().EmitTypedEvent(&types.EventTotalsUpdated{
			PendingAmount:       totals.PendingAmount,
			PendingCount:        totals.PendingCount,
			MintedAmount:        totals.MintedAmount,
			MintedCount:         totals.MintedCount,
			VoidedAmount:        totals.VoidedAmount,
			VoidedCount:         totals.VoidedCount,
			RefundPendingAmount: totals.RefundPendingAmount,
			RefundPendingCount:  totals.RefundPendingCount,
			RefundedAmount:      totals.RefundedAmount,
			RefundedCount:       totals.RefundedCount,
			RefundedInEpoch:     usage.Amount,
		}); err != nil {
			return err
		}
	}

	if before == nil {
		if err := k.history.Set(ctx, after.ID); err != nil {
			return err
		}
	}

	oldDispute := before != nil && before.Settlement.Hold != types.HoldNone
	newDispute := after.Settlement.Hold != types.HoldNone
	if oldDispute != newDispute {
		var err error
		if newDispute {
			err = k.disputes.Set(ctx, after.ID)
		} else {
			err = k.disputes.Remove(ctx, after.ID)
		}

		if err != nil {
			return err
		}
	}

	oldKey, oldQueued := collections.Triple[uint64, int64, types.ID]{}, false
	if before != nil {
		oldKey, oldQueued = refundKey(*before)
	}

	newKey, newQueued := refundKey(after)
	same := oldQueued && newQueued && oldKey.K1() == newKey.K1() && oldKey.K2() == newKey.K2()
	if oldQueued && !same {
		if err := k.refunds.Remove(ctx, oldKey); err != nil {
			return err
		}
	}

	if newQueued && !same {
		if err := k.refunds.Set(ctx, newKey); err != nil {
			return err
		}
	}

	return k.settlements.Set(ctx, types.NewSettlementID(after.Status, after.ID), after.Settlement)
}

func (k *keeper) refundHead(ctx context.Context, current uint64) (types.ID, bool, error) {
	var best collections.Triple[uint64, int64, types.ID]
	found := false
	for _, scope := range []uint64{types.GovernanceRefundScope, current} {
		it, err := k.refunds.Iterate(ctx, collections.NewPrefixedTripleRange[uint64, int64, types.ID](scope))
		if err != nil {
			return types.ID{}, false, err
		}

		if it.Valid() {
			key, err := it.Key()
			if err != nil {
				return types.ID{}, false, errors.Join(err, it.Close())
			}

			earlier := !found || key.K2() < best.K2()
			if found && key.K2() == best.K2() {
				a, b := make([]byte, types.IDKeySize), make([]byte, types.IDKeySize)
				if _, err = types.IDKey.Encode(a, key.K3()); err != nil {
					return types.ID{}, false, errors.Join(err, it.Close())
				}

				if _, err = types.IDKey.Encode(b, best.K3()); err != nil {
					return types.ID{}, false, errors.Join(err, it.Close())
				}

				earlier = string(a) < string(b)
			}

			if earlier {
				best, found = key, true
			}
		}

		if err = it.Close(); err != nil {
			return types.ID{}, false, err
		}
	}

	if !found {
		return types.ID{}, false, nil
	}

	return best.K3(), true, nil
}
