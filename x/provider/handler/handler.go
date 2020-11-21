package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	mkeeper "github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

// NewHandler returns a handler for "provider" type messages.
func NewHandler(keeper keeper.Keeper, mkeeper mkeeper.Keeper) sdk.Handler {
	ms := NewMsgServerImpl(keeper, mkeeper)

	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateProvider:
			res, err := ms.CreateProvider(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgUpdateProvider:
			res, err := ms.UpdateProvider(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		case *types.MsgDeleteProvider:
			res, err := ms.DeleteProvider(sdk.WrapSDKContext(ctx), msg)
			return sdk.WrapServiceResult(ctx, res, err)

		default:
			return nil, sdkerrors.Wrapf(sdkerrors.ErrUnknownRequest, "unrecognized bank message type: %T", msg)
		}
	}
}

// var (
// 	// ErrInternal defines registered error code for internal error
// 	ErrInternal = sdkerrors.Register(types.ModuleName, 10, "internal error")
// )
//
// func handleMsgCreate(ctx sdk.Context, keeper keeper.Keeper, msg *types.MsgCreateProvider) (*sdk.Result, error) {
// 	owner, err := sdk.AccAddressFromBech32(msg.Owner)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	if _, ok := keeper.Get(ctx, owner); ok {
// 		return nil, errors.Wrapf(types.ErrProviderExists, "id: %s", msg.Owner)
// 	}
//
// 	if err := msg.Attributes.Validate(); err != nil {
// 		return nil, err
// 	}
//
// 	if err := keeper.Create(ctx, types.Provider(*msg)); err != nil {
// 		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
// 	}
//
// 	return &sdk.Result{
// 		Events: ctx.EventManager().ABCIEvents(),
// 	}, nil
// }
//
// func handleMsgUpdate(ctx sdk.Context, keeper keeper.Keeper, mkeeper mkeeper.Keeper, msg *types.MsgUpdateProvider) (*sdk.Result, error) {
// 	owner, err := sdk.AccAddressFromBech32(msg.Owner)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	prov, found := keeper.Get(ctx, owner)
// 	if !found {
// 		return nil, errors.Wrapf(types.ErrProviderNotFound, "id: %s", msg.Owner)
// 	}
//
// 	if err := msg.Attributes.Validate(); err != nil {
// 		return nil, err
// 	}
//
// 	// all filtering code below is madness!. should make an index to not melt the cpu
// 	// TODO: use WithActiveLeases, filter by lease.Provider
// 	mkeeper.WithLeases(ctx, func(lease mtypes.Lease) bool {
// 		if prov.Owner == lease.ID().Provider && (lease.State == mtypes.LeaseActive) {
// 			var order mtypes.Order
// 			order, found = mkeeper.GetOrder(ctx, lease.ID().OrderID())
// 			if !found {
// 				err = errors.Wrapf(ErrInternal,
// 					"order \"%s\" for lease \"%s\" has not been found",
// 					order.ID(),
// 					lease.ID())
// 				return true
// 			}
//
// 			// fixme(troian) do we need to check audited attributes here?
// 			if !order.MatchAttributes(msg.Attributes) {
// 				err = types.ErrIncompatibleAttributes
// 				return true
// 			}
// 		}
// 		return false
// 	})
//
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	if err := keeper.Update(ctx, types.Provider(*msg)); err != nil {
// 		return nil, sdkerrors.Wrapf(ErrInternal, "err: %v", err)
// 	}
//
// 	return &sdk.Result{
// 		Events: ctx.EventManager().ABCIEvents(),
// 	}, nil
// }
//
// func handleMsgDelete(ctx sdk.Context, keeper keeper.Keeper, msg *types.MsgDeleteProvider) (*sdk.Result, error) {
// 	owner, err := sdk.AccAddressFromBech32(msg.Owner)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	if _, ok := keeper.Get(ctx, owner); !ok {
// 		return nil, types.ErrProviderNotFound
// 	}
//
// 	// TODO: cancel leases
// 	return nil, sdkerrors.Wrapf(ErrInternal, "NOTIMPLEMENTED")
// }
