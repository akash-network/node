package handler

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	types "pkg.akt.dev/go/node/staking/v1beta3"

	"pkg.akt.dev/node/x/staking/keeper"
)

var _ types.MsgServer = msgServer{}

type msgServer struct {
	keeper keeper.IKeeper
}

// NewMsgServerImpl returns an implementation of the akash staking MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(k keeper.IKeeper) types.MsgServer {
	return &msgServer{
		keeper: k,
	}
}

func (ms msgServer) UpdateParams(goCtx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if ms.keeper.GetAuthority() != req.Authority {
		return nil, govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", ms.keeper.GetAuthority(), req.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := ms.keeper.SetParams(ctx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}
