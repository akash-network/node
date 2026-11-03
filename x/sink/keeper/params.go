package keeper

import (
	"errors"
	"math"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

func (k *keeper) UpdateParams(ctx sdk.Context, msg *types.MsgUpdateParams) error {
	cache, write := ctx.CacheContext()

	p := msg.Params
	for _, member := range p.Reporters {
		addr, err := sdk.AccAddressFromBech32(member)
		if err != nil {
			return err
		}

		if _, module := k.accounts.GetAccount(cache, addr).(sdk.ModuleAccountI); module {
			return types.ErrInvalidReporters.Wrap("reporters must be ordinary accounts")
		}
	}

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

	policy, err := k.GetCurrentPolicy(cache)
	if err != nil {
		return err
	}

	if !policy.SameRule(p.Policy(policy.Version)) {
		if policy.Version == math.MaxUint64 {
			return types.ErrOverflow.Wrap("policy version")
		}

		next := p.Policy(policy.Version + 1)
		if err = k.policies.Set(cache, next.Version, next); err != nil {
			return err
		}

		if err = k.currentPolicyVersion.Set(cache, next.Version); err != nil {
			return err
		}
	}

	if err = k.params.Set(cache, p); err != nil {
		return err
	}

	write()

	return nil
}
