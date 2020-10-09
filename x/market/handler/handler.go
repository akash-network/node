package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

// NewHandler returns a handler for "market" type messages
func NewHandler(keepers Keepers) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case *types.MsgCreateBid:
			return handleMsgCreateBid(ctx, keepers, msg)
		case *types.MsgCloseBid:
			return handleMsgCloseBid(ctx, keepers, msg)
		case *types.MsgCloseOrder:
			return handleMsgCloseOrder(ctx, keepers, msg)
		default:
			return nil, sdkerrors.ErrUnknownRequest
		}
	}
}

func handleMsgCreateBid(ctx sdk.Context, keepers Keepers, msg *types.MsgCreateBid) (*sdk.Result, error) {
	order, found := keepers.Market.GetOrder(ctx, msg.Order)
	if !found {
		return nil, types.ErrInvalidOrder
	}

	if err := order.ValidateCanBid(); err != nil {
		return nil, err
	}

	if !msg.Price.IsValid() {
		return nil, types.ErrBidInvalidPrice
	}

	if order.Price().IsLT(msg.Price) {
		return nil, types.ErrBidOverOrder
	}

	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, types.ErrEmptyProvider
	}

	var prov ptypes.Provider
	if prov, found = keepers.Provider.Get(ctx, provider); !found {
		return nil, types.ErrEmptyProvider
	}

	if !order.MatchAttributes(prov.Attributes) {
		return nil, types.ErrAttributeMismatch
	}

	if _, err := keepers.Market.CreateBid(ctx, msg.Order, provider, msg.Price); err != nil {
		return nil, err
	}

	return &sdk.Result{
		Events: ctx.EventManager().ABCIEvents(),
	}, nil
}

func handleMsgCloseBid(ctx sdk.Context, keepers Keepers, msg *types.MsgCloseBid) (*sdk.Result, error) {
	bid, found := keepers.Market.GetBid(ctx, msg.BidID)
	if !found {
		return nil, types.ErrUnknownBid
	}

	order, found := keepers.Market.GetOrder(ctx, msg.BidID.OrderID())
	if !found {
		return nil, types.ErrUnknownOrderForBid
	}

	if bid.State == types.BidOpen {
		keepers.Market.OnBidClosed(ctx, bid)
		return &sdk.Result{
			Events: ctx.EventManager().ABCIEvents(),
		}, nil
	}

	lease, found := keepers.Market.GetLease(ctx, types.LeaseID(msg.BidID))
	if !found {
		return nil, types.ErrUnknownLeaseForBid
	}

	if lease.State != types.LeaseActive {
		return nil, types.ErrLeaseNotActive
	}

	if bid.State != types.BidMatched {
		return nil, types.ErrBidNotMatched
	}

	keepers.Market.OnBidClosed(ctx, bid)
	keepers.Market.OnLeaseClosed(ctx, lease)
	keepers.Market.OnOrderClosed(ctx, order)
	keepers.Deployment.OnLeaseClosed(ctx, order.ID().GroupID())

	return &sdk.Result{
		Events: ctx.EventManager().ABCIEvents(),
	}, nil
}

func handleMsgCloseOrder(ctx sdk.Context, keepers Keepers, msg *types.MsgCloseOrder) (*sdk.Result, error) {
	order, found := keepers.Market.GetOrder(ctx, msg.OrderID)
	if !found {
		return nil, types.ErrUnknownOrder
	}

	lease, found := keepers.Market.LeaseForOrder(ctx, order.ID())
	if !found {
		return nil, types.ErrNoLeaseForOrder
	}

	keepers.Market.OnOrderClosed(ctx, order)
	keepers.Market.OnLeaseClosed(ctx, lease)
	keepers.Deployment.OnLeaseClosed(ctx, order.ID().GroupID())
	return &sdk.Result{
		Events: ctx.EventManager().ABCIEvents(),
	}, nil
}
