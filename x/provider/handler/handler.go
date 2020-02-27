package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

// NewHandler returns a handler for "provider" type messages.
func NewHandler(keeper keeper.Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case types.MsgCreate:
			return handleMsgCreate(ctx, keeper, msg)
		case types.MsgUpdate:
			return handleMsgUpdate(ctx, keeper, msg)
		case types.MsgDelete:
			return handleMsgDelete(ctx, keeper, msg)
		default:
			return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unrecognized bank message type: %T", msg)
		}
	}
}

var (
	ErrInternal = sdkerrors.Register(types.ModuleName, 10, "internal error")
)

func handleMsgCreate(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgCreate) (*sdk.Result, error) {
	if err := keeper.Create(ctx, types.Provider(msg)); err != nil {
		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
	}

	return &sdk.Result{
		Events: ctx.EventManager().Events(),
	}, nil
}

func handleMsgUpdate(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgUpdate) (*sdk.Result, error) {
	if err := keeper.Update(ctx, types.Provider(msg)); err != nil {
		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
	}
	// TODO: cancel now-invalid leases?
	return &sdk.Result{
		Events: ctx.EventManager().Events(),
	}, nil
}

func handleMsgDelete(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgDelete) (*sdk.Result, error) {
	// TODO: validate exists
	// TODO: cancel leases
	return &sdk.Result{}, sdkerrors.Wrapf(ErrInternal, "NOTIMPLEMENTED", "")
}
