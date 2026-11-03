package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

func (k Keeper) InitGenesis(ctx sdk.Context, g *types.GenesisState) error {
	if g == nil {
		return types.ErrInvalidGenesis.Wrap("nil genesis")
	}

	if err := g.Validate(); err != nil {
		return err
	}

	it := ctx.KVStore(k.skey).Iterator(nil, nil)
	nonempty := it.Valid()

	if err := it.Close(); err != nil {
		return err
	}

	if nonempty {
		return types.ErrInvalidGenesis.Wrap("sink store is not empty")
	}

	epoch, err := types.EpochOf(ctx.BlockTime())
	if err != nil {
		return err
	}

	if g.SendUsage.Epoch > epoch || g.RefundUsage.Epoch > epoch {
		return types.ErrInvalidGenesis.Wrap("future epoch usage")
	}

	firstHeight := max(int64(1), ctx.BlockHeight())
	for _, t := range g.Transfers {
		if t.ID.Height >= firstHeight {
			return types.ErrInvalidGenesis.Wrap("transfer height must precede first executable height")
		}
	}

	cache, write := ctx.CacheContext()
	if account := k.accounts.GetAccount(cache, Address()); account != nil {
		module, ok := account.(sdk.ModuleAccountI)
		if !ok || module.GetName() != types.ModuleName {
			return types.ErrInvalidGenesis.Wrap("sink address must be its module account")
		}
	}

	k.accounts.GetModuleAccount(cache, types.ModuleName)

	if err = k.params.Set(cache, g.Params); err != nil {
		return err
	}

	if err = k.totals.Set(cache, types.DefaultTotals()); err != nil {
		return err
	}

	for _, p := range g.Policies {
		for _, member := range p.Members {
			addr, err := sdk.AccAddressFromBech32(member)
			if err != nil {
				return err
			}

			if _, module := k.accounts.GetAccount(cache, addr).(sdk.ModuleAccountI); module {
				return types.ErrInvalidReporters.Wrap("reporters must be ordinary accounts")
			}
		}

		if err = k.policies.Set(cache, p.Version, p); err != nil {
			return err
		}
	}

	if err = k.sendUsage.Set(cache, g.SendUsage); err != nil {
		return err
	}

	if err = k.refundUsage.Set(cache, g.RefundUsage); err != nil {
		return err
	}

	for _, t := range g.Transfers {
		if err = k.transfers.Set(cache, t.ID, t.Data); err != nil {
			return err
		}

		if t.VoidOrdered {
			if err = k.voids.Set(cache, t.ID, types.VoidOrder{}); err != nil {
				return err
			}
		}

		if err = k.put(cache, t.ID, t.Data.Amount, nil, t.Settlement); err != nil {
			return err
		}
	}

	for _, a := range g.Accounts {
		owner, err := sdk.AccAddressFromBech32(a.Owner)
		if err != nil {
			return err
		}

		if err = k.usage.Set(cache, owner, a.Usage); err != nil {
			return err
		}
	}

	totals, err := k.totals.Get(cache)
	if err != nil {
		return err
	}

	if !totals.Equal(g.Totals) {
		return types.ErrInvalidGenesis.Wrap("recomputed totals differ")
	}

	if err = k.CheckBalance(cache); err != nil {
		return err
	}

	if err = k.sequence.Set(cache, 0); err != nil {
		return err
	}

	write()

	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error) {
	g := &types.GenesisState{SchemaVersion: types.SchemaVersion}
	var err error
	g.Params, err = k.params.Get(ctx)
	if err != nil {
		return nil, err
	}

	g.Totals, err = k.totals.Get(ctx)
	if err != nil {
		return nil, err
	}

	g.Policies, err = k.allPolicies(ctx)
	if err != nil {
		return nil, err
	}

	g.SendUsage, err = k.sendUsage.Get(ctx)
	if err != nil {
		return nil, err
	}

	g.RefundUsage, err = k.refundUsage.Get(ctx)
	if err != nil {
		return nil, err
	}

	err = k.transfers.Walk(ctx, nil, func(id types.ID, _ types.RecordData) (bool, error) {
		t, err := k.GetTransfer(ctx, id)
		if err != nil {
			return true, err
		}

		g.Transfers = append(g.Transfers, t)

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	err = k.usage.Walk(ctx, nil, func(owner sdk.AccAddress, u types.AccountUsage) (bool, error) {
		g.Accounts = append(g.Accounts, types.GenesisAccount{Owner: owner.String(), Usage: u})
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	err = k.settlements.Walk(ctx, nil, func(id types.ID, _ types.SettlementData) (bool, error) {
		return k.requireRecord(ctx, id)
	})
	if err != nil {
		return nil, err
	}

	err = k.voids.Walk(ctx, nil, func(id types.ID, _ types.VoidOrder) (bool, error) {
		return k.requireRecord(ctx, id)
	})
	if err != nil {
		return nil, err
	}

	err = k.policies.Walk(ctx, nil, func(v uint64, p types.ReporterPolicy) (bool, error) {
		if v != p.Version {
			return true, types.ErrInternal.Wrap("policy key mismatch")
		}

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	if err = g.Validate(); err != nil {
		return nil, err
	}

	if err = k.CheckBalance(ctx); err != nil {
		return nil, err
	}

	return g, nil
}

func (k Keeper) requireRecord(ctx context.Context, id types.ID) (bool, error) {
	exists, err := k.transfers.Has(ctx, id)
	if err != nil {
		return true, err
	}

	if !exists {
		return true, types.ErrInternal.Wrapf("orphaned sink state for %s", id)
	}

	return false, nil
}
