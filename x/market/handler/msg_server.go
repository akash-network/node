package handler

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

type msgServer struct {
	keepers Keepers
}

// NewMsgServerImpl returns an implementation of the market MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(k Keepers) types.MsgServer {
	return &msgServer{keepers: k}
}

var _ types.MsgServer = msgServer{}

func (ms msgServer) CreateBid(goCtx context.Context, msg *types.MsgCreateBid) (*types.MsgCreateBidResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	order, found := ms.keepers.Market.GetOrder(ctx, msg.Order)
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
	if prov, found = ms.keepers.Provider.Get(ctx, provider); !found {
		return nil, types.ErrEmptyProvider
	}

	if !order.MatchAttributes(prov.Attributes) {
		return nil, types.ErrAttributeMismatch
	}

	if _, err := ms.keepers.Market.CreateBid(ctx, msg.Order, provider, msg.Price); err != nil {
		return nil, err
	}

	return &types.MsgCreateBidResponse{}, nil
}

func (ms msgServer) CloseBid(goCtx context.Context, msg *types.MsgCloseBid) (*types.MsgCloseBidResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	bid, found := ms.keepers.Market.GetBid(ctx, msg.BidID)
	if !found {
		return nil, types.ErrUnknownBid
	}

	order, found := ms.keepers.Market.GetOrder(ctx, msg.BidID.OrderID())
	if !found {
		return nil, types.ErrUnknownOrderForBid
	}

	if bid.State == types.BidOpen {
		ms.keepers.Market.OnBidClosed(ctx, bid)
		return &types.MsgCloseBidResponse{}, nil
	}

	lease, found := ms.keepers.Market.GetLease(ctx, types.LeaseID(msg.BidID))
	if !found {
		return nil, types.ErrUnknownLeaseForBid
	}

	if lease.State != types.LeaseActive {
		return nil, types.ErrLeaseNotActive
	}

	if bid.State != types.BidMatched {
		return nil, types.ErrBidNotMatched
	}

	ms.keepers.Market.OnBidClosed(ctx, bid)
	ms.keepers.Market.OnLeaseClosed(ctx, lease)
	ms.keepers.Market.OnOrderClosed(ctx, order)
	ms.keepers.Deployment.OnLeaseClosed(ctx, order.ID().GroupID())

	return &types.MsgCloseBidResponse{}, nil
}

func (ms msgServer) CloseOrder(goCtx context.Context, msg *types.MsgCloseOrder) (*types.MsgCloseOrderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	order, found := ms.keepers.Market.GetOrder(ctx, msg.OrderID)
	if !found {
		return nil, types.ErrUnknownOrder
	}

	lease, found := ms.keepers.Market.LeaseForOrder(ctx, order.ID())
	if !found {
		return nil, types.ErrNoLeaseForOrder
	}

	ms.keepers.Market.OnOrderClosed(ctx, order)
	ms.keepers.Market.OnLeaseClosed(ctx, lease)
	ms.keepers.Deployment.OnLeaseClosed(ctx, order.ID().GroupID())

	return &types.MsgCloseOrderResponse{}, nil
}
