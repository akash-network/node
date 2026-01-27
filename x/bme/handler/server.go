package handler

import (
	"context"

	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"pkg.akt.dev/go/sdkutil"

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
	src, err := sdk.AccAddressFromBech32(msg.Owner)
	if err != nil {
		return nil, errors.Wrapf(sdkerrors.ErrInvalidAddress, "invalid owner address: %s", err)
	}

	dst, err := sdk.AccAddressFromBech32(msg.To)
	if err != nil {
		return nil, errors.Wrapf(sdkerrors.ErrInvalidAddress, "invalid to address: %s", err)
	}

	err = msg.CoinsToBurn.Validate()
	if err != nil {
		return nil, errors.Wrapf(sdkerrors.ErrInvalidCoins, "invalid coins: %s", err)
	}

	id, err := ms.bme.RequestBurnMint(ctx, src, dst, msg.CoinsToBurn, msg.DenomToMint)
	if err != nil {
		return nil, err
	}
	resp := &types.MsgBurnMintResponse{
		ID: id,
	}

	return resp, nil
}

func (ms msgServer) MintACT(ctx context.Context, msg *types.MsgMintACT) (*types.MsgMintACTResponse, error) {
	r, err := ms.BurnMint(ctx, &types.MsgBurnMint{
		Owner:       msg.Owner,
		To:          msg.To,
		CoinsToBurn: msg.CoinsToBurn,
		DenomToMint: sdkutil.DenomUact,
	})
	if err != nil {
		return nil, err
	}

	resp := &types.MsgMintACTResponse{
		ID: r.ID,
	}

	return resp, nil
}

func (ms msgServer) BurnACT(ctx context.Context, msg *types.MsgBurnACT) (*types.MsgBurnACTResponse, error) {
	r, err := ms.BurnMint(ctx, &types.MsgBurnMint{
		Owner:       msg.Owner,
		To:          msg.To,
		CoinsToBurn: msg.CoinsToBurn,
		DenomToMint: sdkutil.DenomUakt,
	})
	if err != nil {
		return nil, err
	}

	resp := &types.MsgBurnACTResponse{
		ID: r.ID,
	}

	return resp, nil
}
