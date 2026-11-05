package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// GuardianPause lets params.pause_guardian turn off deposits, refunds or both.
// It can only clear params.enabled and params.refunds_enabled; enabling
// either again, or changing the guardian, takes governance. Params are stored
// without revalidation. Clearing these switches only relaxes the rules, and a
// pause must not fail because a stored value predates a newer rule.
func (k *keeper) GuardianPause(ctx sdk.Context, msg *types.MsgGuardianPause) error {
	cache, write := ctx.CacheContext()
	p, err := k.params.Get(cache)
	if err != nil {
		return err
	}

	if msg.Deposits {
		p.Enabled = false
	}

	if msg.Refunds {
		p.RefundsEnabled = false
	}

	if err = k.params.Set(cache, p); err != nil {
		return err
	}

	if err = cache.EventManager().EmitTypedEvent(&types.EventGuardianPaused{
		Guardian:       msg.Guardian,
		Deposits:       msg.Deposits,
		Refunds:        msg.Refunds,
		Enabled:        p.Enabled,
		RefundsEnabled: p.RefundsEnabled,
	}); err != nil {
		return err
	}

	write()

	return nil
}
