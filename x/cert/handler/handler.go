package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/akash-network/node/x/cert/keeper"
	types "github.com/akash-network/node/x/cert/types/v1beta2"
)

// NewHandler returns a handler for "provider" type messages.
func NewHandler(keeper keeper.Keeper) sdk.Handler {
	ms := NewMsgServerImpl(keeper)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateCertificate:
			res, err := ms.CreateCertificate(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)
		case *types.MsgRevokeCertificate:
			res, err := ms.RevokeCertificate(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)
		}

		return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unrecognized message type: %T", msg)
	}
}
