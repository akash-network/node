package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

func refundKey(s types.SettlementData, id types.ID) (collections.Triple[uint64, string, types.ID], bool) {
	scope, ok := s.RefundScope()
	if !ok || s.ChallengeEnd == nil {
		return collections.Triple[uint64, string, types.ID]{}, false
	}

	return collections.Join3(scope, string(sdk.FormatTimeBytes(*s.ChallengeEnd)), id), true
}

func (k Keeper) put(ctx sdk.Context, id types.ID, amount sdk.Coin, before *types.SettlementData, after types.SettlementData) error {
	policies, err := k.referencedPolicies(ctx, after)
	if err != nil {
		return err
	}

	if err = after.Validate(policies); err != nil {
		return err
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
			if err = totals.Sub(before.Status, amount); err != nil {
				return types.ErrInternal.Wrap("invalid settlement accounting")
			}

			if err = k.byStatus.Remove(ctx, settlementID(before.Status, id)); err != nil {
				return err
			}
		}

		if err = totals.Add(after.Status, amount); err != nil {
			return err
		}

		if err = k.byStatus.Set(ctx, settlementID(after.Status, id)); err != nil {
			return err
		}

		if err = k.totals.Set(ctx, totals); err != nil {
			return err
		}

		if err = ctx.EventManager().EmitTypedEvent(&types.EventTotalsUpdated{Totals: totals}); err != nil {
			return err
		}
	}

	if before == nil {
		if err = k.history.Set(ctx, id); err != nil {
			return err
		}
	}

	oldDispute := before != nil && before.Hold != types.HoldNone
	newDispute := after.Hold != types.HoldNone
	if oldDispute != newDispute {
		if newDispute {
			err = k.disputes.Set(ctx, id)
		} else {
			err = k.disputes.Remove(ctx, id)
		}

		if err != nil {
			return err
		}
	}

	oldKey, oldQueued := collections.Triple[uint64, string, types.ID]{}, false
	if before != nil {
		oldKey, oldQueued = refundKey(*before, id)
	}

	newKey, newQueued := refundKey(after, id)
	same := oldQueued && newQueued && oldKey.K1() == newKey.K1() && oldKey.K2() == newKey.K2()
	if oldQueued && !same {
		if err = k.refunds.Remove(ctx, oldKey); err != nil {
			return err
		}
	}

	if newQueued && !same {
		if err = k.refunds.Set(ctx, newKey); err != nil {
			return err
		}
	}

	return k.settlements.Set(ctx, id, after)
}

func (k Keeper) refundHead(ctx context.Context, current uint64) (types.ID, bool, error) {
	var best collections.Triple[uint64, string, types.ID]
	found := false
	for _, scope := range []uint64{types.GovernanceRefundScope, current} {
		it, err := k.refunds.Iterate(ctx, collections.NewPrefixedTripleRange[uint64, string, types.ID](scope))
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
				best = key
				found = true
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
