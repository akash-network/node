package keeper

import (
	"math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/sink/v1"
)

func (k Keeper) UpdateParams(ctx sdk.Context, msg *types.MsgUpdateParams) error {
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
		for _, status := range []types.Status{types.StatusTransferPending, types.StatusRefundPending} {
			err = k.byStatus.Walk(cache, statusRange(status), func(key types.SettlementID) (bool, error) {
				data, err := k.transfers.Get(cache, transferID(key))
				if err != nil {
					return true, err
				}

				if data.Amount.IsGT(p.MaxEpochRefund) {
					return true, types.ErrInvalidParam.Wrap("refund cap below unpaid transfer amount")
				}

				return false, nil
			})
			if err != nil {
				return err
			}
		}
	}

	policy, err := k.currentPolicy(cache)
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
	}

	if err = k.params.Set(cache, p); err != nil {
		return err
	}

	write()

	return nil
}
