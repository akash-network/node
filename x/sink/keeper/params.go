package keeper

import (
	"errors"
	"math"
	"slices"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

func (k *keeper) SetParams(ctx sdk.Context, p types.Params) error {
	cache, write := ctx.CacheContext()

	old, err := k.params.Get(cache)
	if err != nil {
		return err
	}

	if p.MaxEpochRefund.IsLT(old.MaxEpochRefund) {
		for _, status := range []types.Status{types.StatusTransferPending, types.StatusVoided, types.StatusRefundPending} {
			err = func() (err error) {
				prefix, err := SettlementIDKey.ToPrefix(types.SettlementID{Status: status})
				if err != nil {
					return err
				}

				it, err := k.settlements.IterateRaw(cache, prefix, storetypes.PrefixEndBytes(prefix), collections.OrderAscending)
				if err != nil {
					return err
				}
				defer func() {
					err = errors.Join(err, it.Close())
				}()

				for ; it.Valid(); it.Next() {
					entry, err := it.KeyValue()
					if err != nil {
						return err
					}

					data, err := k.transfers.Get(cache, *entry.Key.GetID())
					if err != nil {
						return err
					}

					if data.Amount.IsGT(p.MaxEpochRefund) {
						return types.ErrInvalidParam.Wrap("refund cap below unpaid transfer amount")
					}
				}

				return nil
			}()
			if err != nil {
				return err
			}
		}
	}

	newMembers := make([]string, len(p.Reporters))
	copy(newMembers, p.Reporters)

	slices.Sort(newMembers)

	version, err := k.policySequence.Peek(cache)
	if err != nil {
		return err
	}

	var currPolicy types.ReporterPolicy
	if version > 0 {
		currPolicy, err = k.GetCurrentPolicy(cache)
		if err != nil {
			return err
		}
	}

	currentMembers := slices.Clone(currPolicy.Members)
	slices.Sort(currentMembers)
	if version == 0 || !slices.Equal(currentMembers, newMembers) || !p.ReportersThreshold.Equal(currPolicy.Threshold) {
		if version == math.MaxUint64 {
			return types.ErrOverflow.Wrap("policy version")
		}

		version++
		newPolicy := types.ReporterPolicy{
			Version:   version,
			Members:   newMembers,
			Threshold: p.ReportersThreshold,
		}
		if err = k.policies.Set(cache, version, newPolicy); err != nil {
			return err
		}

		if err = k.policySequence.Set(cache, version); err != nil {
			return err
		}
	}

	if err = k.params.Set(cache, p); err != nil {
		return err
	}

	write()

	return nil
}
