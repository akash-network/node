package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	mkeeper "github.com/akash-network/node/x/market/keeper"
	"github.com/akash-network/node/x/provider/keeper"
)

// NewHandler returns a handler for "provider" type messages.
func NewHandler(keeper keeper.IKeeper, mkeeper mkeeper.IKeeper) sdk.Handler {
	ms := NewMsgServerImpl(keeper, mkeeper)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateProvider:
			res, err := ms.CreateProvider(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgUpdateProvider:
			res, err := ms.UpdateProvider(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgDeleteProvider:
			res, err := ms.DeleteProvider(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		default:
			return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unrecognized bank message type: %T", msg)
		}
	}
}
