package handler

import (
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	v1 "pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/x/deployment/keeper"
)

// NewHandler returns a handler for "deployment" type messages
func NewHandler(keeper keeper.IKeeper, mkeeper MarketKeeper, ekeeper EscrowKeeper, authzKeeper AuthzKeeper) baseapp.MsgServiceHandler {
	ms := NewServer(keeper, mkeeper, ekeeper, authzKeeper)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateDeployment:
			res, err := ms.CreateDeployment(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *v1.MsgDepositDeployment:
			res, err := ms.DepositDeployment(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgUpdateDeployment:
			res, err := ms.UpdateDeployment(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgCloseDeployment:
			res, err := ms.CloseDeployment(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgCloseGroup:
			res, err := ms.CloseGroup(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgPauseGroup:
			res, err := ms.PauseGroup(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgStartGroup:
			res, err := ms.StartGroup(ctx, msg)
			return sdk.WrapServiceResult(ctx, res, err)

		default:
			return nil, sdkerrors.ErrUnknownRequest
		}
	}
}
