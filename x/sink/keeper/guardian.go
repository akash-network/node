package keeper

import (
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/sink/v1"
)

// GuardianPause lets params.pause_guardian turn off deposits, refunds or both.
// It can only clear params.enabled and params.refunds_enabled; enabling either
// again, or changing the guardian, takes governance. Params are stored without
// revalidation: clearing these switches only relaxes the rules, and a pause
// must not fail because a stored value predates a newer rule.
func (k Keeper) GuardianPause(ctx sdk.Context, msg *types.MsgGuardianPause) error {
	if msg == nil {
		return sdkerrors.ErrInvalidRequest.Wrap("nil guardian pause")
	}
	if !msg.Deposits && !msg.Refunds {
		return sdkerrors.ErrInvalidRequest.Wrap("guardian pause must disable deposits, refunds or both")
	}
	p := k.GetParams(ctx)
	if p.PauseGuardian == "" || msg.Guardian != p.PauseGuardian {
		return sdkerrors.ErrUnauthorized.Wrap("signer is not the sink pause guardian")
	}
	if msg.Deposits {
		p.Enabled = false
	}
	if msg.Refunds {
		p.RefundsEnabled = false
	}
	ctx.KVStore(k.key).Set(paramsKey, k.cdc.MustMarshal(&p))
	ctx.EventManager().EmitEvent(sdk.NewEvent("sink_guardian_pause",
		sdk.NewAttribute("guardian", msg.Guardian),
		sdk.NewAttribute("deposits", strconv.FormatBool(msg.Deposits)),
		sdk.NewAttribute("refunds", strconv.FormatBool(msg.Refunds)),
		sdk.NewAttribute("enabled", strconv.FormatBool(p.Enabled)),
		sdk.NewAttribute("refunds_enabled", strconv.FormatBool(p.RefundsEnabled))))
	return nil
}
