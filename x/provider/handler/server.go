package handler

import (
	"context"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/provider/v1beta4"

	mkeeper "pkg.akt.dev/node/x/market/keeper"
	"pkg.akt.dev/node/x/provider/keeper"
)

var (
	// ErrInternal defines registered error code for internal error
	ErrInternal = errorsmod.Register(types.ModuleName, 10, "internal error")
)

type msgServer struct {
	provider keeper.IKeeper
	market   mkeeper.IKeeper
}

// NewMsgServerImpl returns an implementation of the market MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(k keeper.IKeeper, mk mkeeper.IKeeper) types.MsgServer {
	return &msgServer{provider: k, market: mk}
}

var _ types.MsgServer = msgServer{}

func (ms msgServer) CreateProvider(goCtx context.Context, msg *types.MsgCreateProvider) (*types.MsgCreateProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	owner, _ := sdk.AccAddressFromBech32(msg.Owner)

	if _, ok := ms.provider.Get(ctx, owner); ok {
		return nil, types.ErrProviderExists.Wrapf("id: %s", msg.Owner)
	}

	if err := ms.provider.Create(ctx, types.Provider(*msg)); err != nil {
		return nil, ErrInternal.Wrapf("err: %v", err)
	}

	return &types.MsgCreateProviderResponse{}, nil
}

func (ms msgServer) UpdateProvider(goCtx context.Context, msg *types.MsgUpdateProvider) (*types.MsgUpdateProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	err := msg.ValidateBasic()
	if err != nil {
		return nil, err
	}

	owner, _ := sdk.AccAddressFromBech32(msg.Owner)
	_, found := ms.provider.Get(ctx, owner)
	if !found {
		return nil, types.ErrProviderNotFound.Wrapf("id: %s", msg.Owner)
	}

	if err := ms.provider.Update(ctx, types.Provider(*msg)); err != nil {
		return nil, errorsmod.Wrapf(ErrInternal, "err: %v", err)
	}

	return &types.MsgUpdateProviderResponse{}, nil
}

func (ms msgServer) DeleteProvider(goCtx context.Context, msg *types.MsgDeleteProvider) (*types.MsgDeleteProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	owner, err := sdk.AccAddressFromBech32(msg.Owner)
	if err != nil {
		return nil, err
	}

	if _, ok := ms.provider.Get(ctx, owner); !ok {
		return nil, types.ErrProviderNotFound
	}

	// TODO: cancel leases
	return nil, ErrInternal.Wrap("NOTIMPLEMENTED")
}
