package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "github.com/akash-network/akash-api/go/node/audit/v1beta3"

	"github.com/akash-network/node/x/audit/keeper"
)

// NewHandler returns a handler for "provider" type messages.
func NewHandler(keeper keeper.Keeper) sdk.Handler {
	ms := NewMsgServerImpl(keeper)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgSignProviderAttributes:
			res, err := ms.SignProviderAttributes(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)
		case *types.MsgDeleteProviderAttributes:
			res, err := ms.DeleteProviderAttributes(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)
		}

		return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unrecognized message type: %T", msg)
	}
}
