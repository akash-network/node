package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	"github.com/akash-network/node/x/deployment/keeper"
)

// NewHandler returns a handler for "deployment" type messages
func NewHandler(keeper keeper.IKeeper, mkeeper MarketKeeper, ekeeper EscrowKeeper, authzKeeper AuthzKeeper) sdk.Handler {
	ms := NewServer(keeper, mkeeper, ekeeper, authzKeeper)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateDeployment:
			res, err := ms.CreateDeployment(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgDepositDeployment:
			res, err := ms.DepositDeployment(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgUpdateDeployment:
			res, err := ms.UpdateDeployment(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgCloseDeployment:
			res, err := ms.CloseDeployment(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgCloseGroup:
			res, err := ms.CloseGroup(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgPauseGroup:
			res, err := ms.PauseGroup(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgStartGroup:
			res, err := ms.StartGroup(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		default:
			return nil, sdkerrors.ErrUnknownRequest
		}
	}
}
