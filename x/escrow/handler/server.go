package handler

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/escrow/v1"

	"pkg.akt.dev/node/x/escrow/keeper"
)

var _ types.MsgServer = msgServer{}

type msgServer struct {
	keeper      keeper.Keeper
	authzKeeper AuthzKeeper
	bkeeper     BankKeeper
}

// NewServer returns an implementation of the deployment MsgServer interface
// for the provided Keeper.
func NewServer(k keeper.Keeper, authzKeeper AuthzKeeper, bkeeper BankKeeper) types.MsgServer {
	return &msgServer{
		keeper:      k,
		authzKeeper: authzKeeper,
		bkeeper:     bkeeper,
	}
}

func (ms msgServer) AccountDeposit(goCtx context.Context, msg *types.MsgAccountDeposit) (*types.MsgAccountDepositResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deposits, err := ms.keeper.AuthorizeDeposits(ctx, msg)
	if err != nil {
		return &types.MsgAccountDepositResponse{}, err
	}

	if err := ms.keeper.AccountDeposit(ctx, msg.ID, deposits); err != nil {
		return &types.MsgAccountDepositResponse{}, err
	}

	return &types.MsgAccountDepositResponse{}, nil
}
