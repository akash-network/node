package handler

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

func NewHandler(keeper keeper.Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) sdk.Result {
		switch msg := msg.(type) {
		case types.MsgCreate:
			return handleMsgCreate(ctx, keeper, msg)
		case types.MsgUpdate:
			return handleMsgUpdate(ctx, keeper, msg)
		case types.MsgDelete:
			return handleMsgDelete(ctx, keeper, msg)
		default:
			errMsg := fmt.Sprintf("Unrecognized message type: %v", msg.Type())
			return sdk.ErrUnknownRequest(errMsg).Result()
		}
	}
}

func handleMsgCreate(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgCreate) sdk.Result {
	if err := keeper.Create(ctx, types.Provider(msg)); err != nil {
		return sdk.ErrInternal(err.Error()).Result()
	}

	return sdk.Result{
		Events: ctx.EventManager().Events(),
	}
}

func handleMsgUpdate(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgUpdate) sdk.Result {
	if err := keeper.Update(ctx, types.Provider(msg)); err != nil {
		return sdk.ErrInternal(err.Error()).Result()
	}
	// TODO: cancel now-invalid leases?
	return sdk.Result{
		Events: ctx.EventManager().Events(),
	}
}

func handleMsgDelete(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgDelete) sdk.Result {
	// TODO: validate exists
	// TODO: cancel leases
	return sdk.ErrInternal("NOT IMPLEMENTED").Result()
}
