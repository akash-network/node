package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

func (k *keeper) InitGenesis(ctx sdk.Context, g *types.GenesisState) error {
	if g == nil {
		return types.ErrInvalidGenesis.Wrap("nil genesis")
	}

	if err := g.Validate(); err != nil {
		return err
	}

	if err := k.policySequence.Set(ctx, g.PolicySequence); err != nil {
		return err
	}

	for _, p := range g.Policies {
		if err := k.policies.Set(ctx, p.Version, p); err != nil {
			return err
		}
	}

	if err := k.sendUsage.Set(ctx, g.SendUsage); err != nil {
		return err
	}

	if err := k.refundUsage.Set(ctx, g.RefundUsage); err != nil {
		return err
	}

	for _, t := range g.Records {
		if err := k.transfers.Set(ctx, t.ID, t.Data); err != nil {
			return err
		}

		if err := k.setSettlement(ctx, nil, types.TransferState(t)); err != nil {
			return err
		}
	}

	for _, a := range g.Accounts {
		owner, err := sdk.AccAddressFromBech32(a.Owner)
		if err != nil {
			return err
		}

		if err = k.usage.Set(ctx, owner, a.Usage); err != nil {
			return err
		}
	}

	if err := k.totals.Set(ctx, g.Totals); err != nil {
		return err
	}

	if err := k.params.Set(ctx, g.Params); err != nil {
		return err
	}

	if err := k.CheckBalance(ctx); err != nil {
		return err
	}

	return nil
}

func (k *keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error) {
	g := &types.GenesisState{}
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

	g.PolicySequence, err = k.policySequence.Peek(ctx)
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

		g.Records = append(g.Records, types.GenesisRecord(t))

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	err = k.usage.Walk(ctx, nil, func(owner sdk.AccAddress, u types.AccountUsage) (bool, error) {
		g.Accounts = append(g.Accounts, types.GenesisAccount{
			Owner: owner.String(),
			Usage: u,
		})

		return false, nil
	})
	if err != nil {
		return nil, err
	}

	err = k.settlements.Walk(ctx, nil, func(id types.SettlementID, _ types.SettlementData) (bool, error) {
		return k.requireRecord(ctx, *id.GetID())
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

func (k *keeper) requireRecord(ctx context.Context, id types.ID) (bool, error) {
	exists, err := k.transfers.Has(ctx, id)
	if err != nil {
		return true, err
	}

	if !exists {
		return true, types.ErrInternal.Wrapf("orphaned sink state for %s", id.String())
	}

	return false, nil
}
