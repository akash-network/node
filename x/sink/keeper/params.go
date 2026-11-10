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
	if err := p.ValidateBasic(); err != nil {
		return err
	}

	// Params.ValidateBasic does not apply the quorum rule. Settlement writes and
	// export revalidate every stored policy, and policy history is immutable, so
	// a set that fails the rule must never be stored.
	if err := types.ValidateReporters(p.Reporters, p.ReportersThreshold); err != nil {
		return err
	}

	// A module account cannot run a reporter, so listing one would only shrink
	// the set that can reach quorum. An address without an account is allowed:
	// a new reporter key has none until it is funded.
	for _, member := range p.Reporters {
		addr, err := sdk.AccAddressFromBech32(member)
		if err != nil {
			return err
		}

		if _, module := k.accounts.GetAccount(ctx, addr).(sdk.ModuleAccountI); module {
			return types.ErrInvalidReporters.Wrapf("reporter %s is a module account", member)
		}
	}

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

	if version == 0 || !currPolicy.WithinThreshold(p.ReportersThreshold, p.Reporters) {
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
