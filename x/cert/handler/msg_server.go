package handler

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/cert/v1"

	"pkg.akt.dev/node/x/cert/keeper"
)

type msgServer struct {
	keeper keeper.Keeper
}

var _ types.MsgServer = msgServer{}

// NewMsgServerImpl returns an implementation of the market MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(k keeper.Keeper) types.MsgServer {
	return &msgServer{keeper: k}
}

func (m msgServer) CreateCertificate(goCtx context.Context, req *types.MsgCreateCertificate) (*types.MsgCreateCertificateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	owner, err := sdk.AccAddressFromBech32(req.Owner)
	if err != nil {
		return nil, err
	}

	err = m.keeper.CreateCertificate(ctx, owner, req.Cert, req.Pubkey)
	if err != nil {
		return nil, err
	}

	return &types.MsgCreateCertificateResponse{}, nil
}

func (m msgServer) RevokeCertificate(goCtx context.Context, req *types.MsgRevokeCertificate) (*types.MsgRevokeCertificateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	id, err := types.ToCertID(req.ID)
	if err != nil {
		return nil, err
	}

	err = m.keeper.RevokeCertificate(ctx, id)
	if err != nil {
		return nil, err
	}

	return &types.MsgRevokeCertificateResponse{}, nil
}
