package handler

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	types "pkg.akt.dev/go/node/oracle/v1"

	"pkg.akt.dev/node/v2/x/oracle/keeper"
)

var _ types.MsgServer = msgServer{}

type msgServer struct {
	keeper keeper.Keeper
}

// NewMsgServerImpl returns an implementation of the akash staking MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(k keeper.Keeper) types.MsgServer {
	return &msgServer{
		keeper: k,
	}
}

func (ms msgServer) AddPriceEntry(ctx context.Context, req *types.MsgAddPriceEntry) (*types.MsgAddPriceEntryResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	source, err := sdk.AccAddressFromBech32(req.Signer)
	if err != nil {
		return nil, err
	}

	if err := ms.keeper.AddPriceEntry(sctx, source, req.ID, req.Price); err != nil {
		return nil, err
	}

	return &types.MsgAddPriceEntryResponse{}, nil
}

func (ms msgServer) UpdateParams(ctx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if ms.keeper.GetAuthority() != req.Authority {
		return nil, govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", ms.keeper.GetAuthority(), req.Authority)
	}

	sctx := sdk.UnwrapSDKContext(ctx)
	if err := ms.keeper.SetParams(sctx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}
