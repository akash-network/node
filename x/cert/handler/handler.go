package handler

import (
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/cert/v1"

	"pkg.akt.dev/node/x/cert/keeper"
)

// NewHandler returns a handler for "provider" type messages.
func NewHandler(keeper keeper.Keeper) baseapp.MsgServiceHandler {
	ms := NewMsgServerImpl(keeper)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateCertificate:
			res, err := ms.CreateCertificate(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)
		case *types.MsgRevokeCertificate:
			res, err := ms.RevokeCertificate(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)
		}

		return nil, sdkerrors.ErrUnknownRequest.Wrapf("unrecognized message type: %T", msg)
	}
}
