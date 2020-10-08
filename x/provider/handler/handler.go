package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"

	mkeeper "github.com/ovrclk/akash/x/market/keeper"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

// NewHandler returns a handler for "provider" type messages.
func NewHandler(keeper keeper.Keeper, mkeeper mkeeper.Keeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateProvider:
			return handleMsgCreate(ctx, keeper, msg)
		case *types.MsgUpdateProvider:
			return handleMsgUpdate(ctx, keeper, mkeeper, msg)
		case *types.MsgDeleteProvider:
			return handleMsgDelete(ctx, keeper, msg)
		default:
			return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unrecognized bank message type: %T", msg)
		}
	}
}

var (
	// ErrInternal defines registered error code for internal error
	ErrInternal = sdkerrors.Register(types.ModuleName, 10, "internal error")
)

func handleMsgCreate(ctx sdk.Context, keeper keeper.Keeper, msg *types.MsgCreateProvider) (*sdk.Result, error) {
	if _, ok := keeper.Get(ctx, msg.Owner); ok {
		return nil, errors.Wrapf(types.ErrProviderExists, "id: %s", msg.Owner)
	}

	if err := msg.Attributes.Validate(); err != nil {
		return nil, err
	}

	if err := keeper.Create(ctx, types.Provider(*msg)); err != nil {
		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
	}

	return &sdk.Result{
		Events: ctx.EventManager().ABCIEvents(),
	}, nil
}

func handleMsgUpdate(ctx sdk.Context, keeper keeper.Keeper, mkeeper mkeeper.Keeper, msg *types.MsgUpdateProvider) (*sdk.Result, error) {
	prov, found := keeper.Get(ctx, msg.Owner)
	if !found {
		return nil, errors.Wrapf(types.ErrProviderNotFound, "id: %s", msg.Owner)
	}

	if err := msg.Attributes.Validate(); err != nil {
		return nil, err
	}

	var err error

	// all filtering code below is madness!. should make an index to not melt the cpu
	// TODO: use WithActiveLeases, filter by lease.Provider
	mkeeper.WithLeases(ctx, func(lease mtypes.Lease) bool {
		if prov.Owner.Equals(lease.ID().Provider) && (lease.State == mtypes.LeaseActive) {
			var order mtypes.Order
			order, found = mkeeper.GetOrder(ctx, lease.ID().OrderID())
			if !found {
				err = errors.Wrapf(ErrInternal,
					"order \"%s\" for lease \"%s\" has not been found",
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

	if err := keeper.Update(ctx, types.Provider(*msg)); err != nil {
		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
	}

	return &sdk.Result{
		Events: ctx.EventManager().ABCIEvents(),
	}, nil
}

func handleMsgDelete(ctx sdk.Context, keeper keeper.Keeper, msg *types.MsgDeleteProvider) (*sdk.Result, error) {
	if _, ok := keeper.Get(ctx, msg.Owner); !ok {
		return nil, types.ErrProviderNotFound
	}

	// TODO: cancel leases
	return nil, sdkerrors.Wrapf(ErrInternal, "NOTIMPLEMENTED")
}
