package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// ValidateGenesis checks genesis state without store access. Beyond the SDK
// shape checks it requires the reporter policy that params describe: without
// a policy sequence the reporter set is empty, otherwise the policy at the
// sequence exists and matches params. SetParams and every reporter command
// load that policy, so a genesis without it could only be repaired by a chain
// upgrade.
func ValidateGenesis(g *types.GenesisState) error {
	if g == nil {
		return types.ErrInvalidGenesis.Wrap("nil genesis")
	}

	if err := g.Validate(); err != nil {
		return err
	}

	if g.PolicySequence == 0 {
		if len(g.Params.Reporters) > 0 {
			return types.ErrInvalidGenesis.Wrap("reporters require a current reporter policy")
		}

		return nil
	}

	for _, policy := range g.Policies {
		if policy.Version != g.PolicySequence {
			continue
		}

		if !policyMatches(policy, g.Params) {
			return types.ErrInvalidGenesis.Wrap("current reporter policy differs from params")
		}

		return nil
	}

	return types.ErrInvalidGenesis.Wrap("missing current reporter policy")
}

// validateGenesis adds the snapshot checks that need keeper state. Import and
// export apply the same snapshot rules; import also requires an empty store,
// transfer and void-request heights below the first executable height, a
// usable account at the sink address and exact bank backing.
func (k *keeper) validateGenesis(ctx sdk.Context, g *types.GenesisState) error {
	if err := ValidateGenesis(g); err != nil {
		return err
	}

	for _, record := range g.Records {
		if !k.governanceVetoAuthorized(record.Settlement) {
			return types.ErrInvalidGenesis.Wrap("governance veto signer differs from authority")
		}
	}

	epoch, err := k.quotaEpoch(ctx)
	if err != nil {
		return err
	}

	// Account stamps are bounded by the send stamp in GenesisState.Validate.
	// A future stamp fails every deposit, refund and status change until the
	// native epoch catches up, and no message can reset it.
	if g.SendUsage.Epoch > epoch || g.RefundUsage.Epoch > epoch {
		return types.ErrInvalidGenesis.Wrap("future epoch usage")
	}

	return nil
}

// InitGenesis imports a validated snapshot into an empty store. Totals are
// recomputed and compared by GenesisState.Validate, so records are written
// without replaying settlement accounting. All writes share one cache, so a
// rejected import leaves no state behind.
func (k *keeper) InitGenesis(ctx sdk.Context, g *types.GenesisState) error {
	if err := k.validateGenesis(ctx, g); err != nil {
		return err
	}

	if err := k.requireEmptyStore(ctx); err != nil {
		return err
	}

	for _, t := range g.Records {
		if err := requirePriorHeight(t.ID, t.Settlement, ctx.BlockHeight()); err != nil {
			return err
		}
	}

	cache, write := ctx.CacheContext()

	if err := k.installModuleAccount(cache); err != nil {
		return err
	}

	if err := k.policySequence.Set(cache, g.PolicySequence); err != nil {
		return err
	}

	for _, p := range g.Policies {
		if err := k.policies.Set(cache, p.Version, p); err != nil {
			return err
		}
	}

	if err := k.sendUsage.Set(cache, g.SendUsage); err != nil {
		return err
	}

	if err := k.refundUsage.Set(cache, g.RefundUsage); err != nil {
		return err
	}

	for _, t := range g.Records {
		if err := k.transfers.Set(cache, t.ID, t.Data); err != nil {
			return err
		}

		if err := k.setSettlement(cache, nil, types.TransferState(t)); err != nil {
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

	if err := k.totals.Set(cache, g.Totals); err != nil {
		return err
	}

	if err := k.params.Set(cache, g.Params); err != nil {
		return err
	}

	if err := k.CheckBalance(cache); err != nil {
		return err
	}

	write()

	return nil
}

func (k *keeper) requireEmptyStore(ctx sdk.Context) error {
	it := ctx.KVStore(k.skey).Iterator(nil, nil)
	nonempty := it.Valid()
	if err := it.Close(); err != nil {
		return err
	}

	if nonempty {
		return types.ErrInvalidGenesis.Wrap("sink store is not empty")
	}

	return nil
}

// installModuleAccount creates the sink module account, or keeps the one auth
// genesis restored or PrepareInstall adopted. Any other account at the address
// would make the first deposit panic in the bank keeper.
func (k *keeper) installModuleAccount(ctx sdk.Context) error {
	if acc := k.accounts.GetAccount(ctx, Address()); acc != nil {
		module, ok := acc.(sdk.ModuleAccountI)
		if !ok || module.GetName() != types.ModuleName {
			return types.ErrInvalidGenesis.Wrap("sink address must be its module account")
		}
	}

	if k.accounts.GetModuleAccount(ctx, types.ModuleName) == nil {
		return types.ErrInternal.Wrap("sink module account is not registered")
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

	if err = k.validateGenesis(ctx, g); err != nil {
		return nil, err
	}

	if err = k.CheckBalance(ctx); err != nil {
		return nil, err
	}

	return g, nil
}

// ValidateExportHeight applies InitGenesis's height rule to the stored
// transfers: a snapshot for a chain whose genesis has initialHeight may hold
// only transfers and void requests from earlier blocks. Transfer IDs keep their
// deposit height, so a zero-height export of any transfer fails.
func (k *keeper) ValidateExportHeight(ctx sdk.Context, initialHeight int64) error {
	return k.transfers.Walk(ctx, nil, func(id types.ID, _ types.RecordData) (bool, error) {
		t, err := k.GetTransfer(ctx, id)
		if err != nil {
			return true, err
		}

		if err = requirePriorHeight(t.ID, t.Settlement, initialHeight); err != nil {
			return true, err
		}

		return false, nil
	})
}

// requirePriorHeight rejects a transfer that a chain whose genesis has
// initialHeight could not import; CometBFT starts heights 0 and 1 at block 1.
// The allocator restarts every block, so a record at or above the first
// executable height could share its height and sequence with a new deposit,
// and export rejects duplicate positions. Void requests must be older too.
func requirePriorHeight(id types.ID, s types.SettlementData, initialHeight int64) error {
	first := max(int64(1), initialHeight)
	void := s.VoidAuthorization
	if id.Height >= first || void != nil && void.RequestedHeight >= first {
		return types.ErrInvalidGenesis.Wrapf("transfer height must precede first executable height %d: %s", first, id.String())
	}

	return nil
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
