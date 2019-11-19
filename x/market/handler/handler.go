package handler

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/market/types"
)

func NewHandler(keepers Keepers) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) sdk.Result {
		switch msg := msg.(type) {
		case types.MsgCreateBid:
			return handleMsgCreateBid(ctx, keepers, msg)
		case types.MsgCloseBid:
			return handleMsgCloseBid(ctx, keepers, msg)
		case types.MsgCloseOrder:
			return handleMsgCloseOrder(ctx, keepers, msg)
		default:
			errMsg := fmt.Sprintf("Unrecognized message type: %v", msg.Type())
			return sdk.ErrUnknownRequest(errMsg).Result()
		}
	}
}

func handleMsgCreateBid(ctx sdk.Context, keepers Keepers, msg types.MsgCreateBid) sdk.Result {
	order, ok := keepers.Market.GetOrder(ctx, msg.Order)
	if !ok {
		return sdk.ErrInternal("unknown order").Result()
	}

	if err := order.ValidateCanBid(); err != nil {
		return sdk.ErrInternal(err.Error()).Result()
	}

	if order.Price().IsLT(msg.Price) {
		return sdk.ErrInternal("bid price above max order price").Result()
	}

	provider, ok := keepers.Provider.Get(ctx, msg.Provider)
	if !ok {
		return sdk.ErrInternal("unknown provider").Result()
	}

	if !order.MatchAttributes(provider.Attributes) {
		return sdk.ErrInternal("attribute mismatch").Result()
	}

	// TODO: ensure not a current bid from this provider

	keepers.Market.CreateBid(ctx, msg.Order, msg.Provider, msg.Price)

	return sdk.Result{
		Events: ctx.EventManager().Events(),
	}
}

func handleMsgCloseBid(ctx sdk.Context, keepers Keepers, msg types.MsgCloseBid) sdk.Result {
	bid, ok := keepers.Market.GetBid(ctx, msg.BidID)
	if !ok {
		return sdk.ErrInternal("unknown bid").Result()
	}

	lease, ok := keepers.Market.GetLease(ctx, types.LeaseID(msg.BidID))
	if !ok {
		return sdk.ErrInternal("unknown lease for bid").Result()
	}

	order, ok := keepers.Market.GetOrder(ctx, msg.OrderID())
	if !ok {
		return sdk.ErrInternal("unknown order for bid").Result()
	}

	if lease.State != types.LeaseActive {
		return sdk.ErrInternal("lease not active").Result()
	}

	if bid.State != types.BidMatched {
		return sdk.ErrInternal("bid not matched").Result()
	}

	keepers.Market.OnBidClosed(ctx, bid)
	keepers.Market.OnLeaseClosed(ctx, lease)
	keepers.Market.OnOrderClosed(ctx, order)
	keepers.Deployment.OnLeaseClosed(ctx, order.GroupID())

	return sdk.Result{
		Events: ctx.EventManager().Events(),
	}
}

func handleMsgCloseOrder(ctx sdk.Context, keepers Keepers, msg types.MsgCloseOrder) sdk.Result {
	order, ok := keepers.Market.GetOrder(ctx, msg.OrderID)
	if !ok {
		return sdk.ErrInternal("unknown order").Result()
	}

	lease, ok := keepers.Market.LeaseForOrder(ctx, order.ID())
	if !ok {
		return sdk.ErrInternal("no lease for order").Result()
	}
	keepers.Market.OnOrderClosed(ctx, order)
	keepers.Market.OnLeaseClosed(ctx, lease)
	keepers.Deployment.OnLeaseClosed(ctx, order.GroupID())
	return sdk.Result{
		Events: ctx.EventManager().Events(),
	}
}
