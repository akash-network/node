package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

// NewHandler returns a handler for "provider" type messages.
func NewHandler(keeper keeper.Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case types.MsgCreateProvider:
			return handleMsgCreate(ctx, keeper, msg)
		case types.MsgUpdateProvider:
			return handleMsgUpdate(ctx, keeper, msg)
		case types.MsgDeleteProvider:
			return handleMsgDelete(ctx, keeper, msg)
		default:
			return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unrecognized bank message type: %T", msg)
		}
	}
}

var (
	// ErrInternal defines registered error code for internal error
	ErrInternal = sdkerrors.Register(types.ModuleName, 10, "internal error")
)

func handleMsgCreate(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgCreateProvider) (*sdk.Result, error) {
	if err := keeper.Create(ctx, types.Provider(msg)); err != nil {
		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
	}

	return &sdk.Result{
		Events: ctx.EventManager().Events(),
	}, nil
}

func handleMsgUpdate(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgUpdateProvider) (*sdk.Result, error) {
	if _, ok := keeper.Get(ctx, msg.Owner); !ok {
		return nil, errors.Wrapf(types.ErrProviderNotFound, "id: %s", msg.Owner)
	}

	if err := keeper.Update(ctx, types.Provider(msg)); err != nil {
		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
	}
	// TODO: cancel now-invalid leases?
	return &sdk.Result{
		Events: ctx.EventManager().Events(),
	}, nil
}

func handleMsgDelete(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgDeleteProvider) (*sdk.Result, error) {
	// TODO: cancel leases
	return &sdk.Result{}, sdkerrors.Wrapf(ErrInternal, "NOTIMPLEMENTED")
}
