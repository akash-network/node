package handler

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	mtypes "github.com/akash-network/akash-api/go/node/market/v1beta3"

	types "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	mkeeper "github.com/akash-network/node/x/market/keeper"
	"github.com/akash-network/node/x/provider/keeper"
)

var (
	// ErrInternal defines registered error code for internal error
	ErrInternal = sdkerrors.Register(types.ModuleName, 10, "internal error")
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
		return nil, fmt.Errorf("%w: id: %s", types.ErrProviderExists, msg.Owner)
	}

	if err := ms.provider.Create(ctx, types.Provider(*msg)); err != nil {
		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
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
	prov, found := ms.provider.Get(ctx, owner)
	if !found {
		return nil, fmt.Errorf("%w: id: %s", types.ErrProviderNotFound, msg.Owner)
	}

	// all filtering code below is madness!. should make an index to not melt the cpu
	// TODO: use WithActiveLeases, filter by lease.Provider
	ms.market.WithLeases(ctx, func(lease mtypes.Lease) bool {
		if prov.Owner == lease.ID().Provider && (lease.State == mtypes.LeaseActive) {
			var order mtypes.Order
			order, found = ms.market.GetOrder(ctx, lease.ID().OrderID())
			if !found {
				err = fmt.Errorf("%w: order \"%s\" for lease \"%s\" has not been found",
					ErrInternal,
					order.ID(),
					lease.ID())
				return true
			}
			if !order.MatchAttributes(msg.Attributes) {
				err = types.ErrIncompatibleAttributes
				return true
			}
		}
		return false
	})

	if err != nil {
		return nil, err
	}

	if err := ms.provider.Update(ctx, types.Provider(*msg)); err != nil {
		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
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
	return nil, sdkerrors.Wrapf(ErrInternal, "NOTIMPLEMENTED")
}
