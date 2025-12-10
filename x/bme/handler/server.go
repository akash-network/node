package handler

import (
	"context"

	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	types "pkg.akt.dev/go/node/bme/v1"

	bmeimports "pkg.akt.dev/node/v2/x/bme/imports"
	"pkg.akt.dev/node/v2/x/bme/keeper"
)

type msgServer struct {
	bme  keeper.Keeper
	bank bmeimports.BankKeeper
}

func NewMsgServerImpl(keeper keeper.Keeper) types.MsgServer {
	return &msgServer{
		bme: keeper,
	}
}

var _ types.MsgServer = msgServer{}

func (ms msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if ms.bme.GetAuthority() != msg.Authority {
		return nil, errors.Wrapf(govtypes.ErrInvalidSigner, "invalid authority; expected %s, got %s", ms.bme.GetAuthority(), msg.Authority)
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}

	if err := ms.bme.SetParams(sctx, msg.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}

func (ms msgServer) BurnMint(ctx context.Context, msg *types.MsgBurnMint) (*types.MsgBurnMintResponse, error) {
	//sctx := sdk.UnwrapSDKContext(ctx)

	resp := &types.MsgBurnMintResponse{}

	return resp, nil
}
