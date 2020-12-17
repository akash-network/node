package handler

import (
	"context"

	"github.com/cosmos/cosmos-sdk/telemetry"

	sdk "github.com/cosmos/cosmos-sdk/types"

	atypes "github.com/ovrclk/akash/x/audit/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

type msgServer struct {
	keepers Keepers
}

// NewMsgServerImpl returns an implementation of the market MsgServer interface
// for the provided Keeper.
func NewServer(k Keepers) types.MsgServer {
	return &msgServer{keepers: k}
}

var _ types.MsgServer = msgServer{}

func (ms msgServer) CreateBid(goCtx context.Context, msg *types.MsgCreateBid) (*types.MsgCreateBidResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params := ms.keepers.Market.GetParams(ctx)

	minDeposit := params.BidMinDeposit
	if msg.Deposit.Denom != minDeposit.Denom {
		return nil, types.ErrInvalidDeposit
	}
	if minDeposit.Amount.GT(msg.Deposit.Amount) {
		return nil, types.ErrInvalidDeposit
	}

	if ms.keepers.Market.BidCountForOrder(ctx, msg.Order) > params.OrderMaxBids {
		// TODOERR
		return nil, types.ErrInvalidDeposit
	}

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

	provAttr, _ := ms.keepers.Audit.GetProviderAttributes(ctx, provider)

	provAttr = append([]atypes.Provider{{
		Owner:      msg.Provider,
		Attributes: prov.Attributes,
	}}, provAttr...)

	if !order.MatchRequirements(provAttr) {
		return nil, types.ErrAttributeMismatch
	}

	bid, err := ms.keepers.Market.CreateBid(ctx, msg.Order, provider, msg.Price)
	if err != nil {
		return nil, err
	}

	// create escrow account for this bid
	if err := ms.keepers.Escrow.AccountCreate(ctx,
		types.EscrowAccountForBid(bid.ID()),
		provider,
		msg.Deposit); err != nil {
		return &types.MsgCreateBidResponse{}, err
	}

	telemetry.IncrCounter(1.0, "akash.bids")
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

	if bid.State != types.BidActive {
		return nil, types.ErrBidNotActive
	}

	ms.keepers.Market.OnBidClosed(ctx, bid)
	ms.keepers.Market.OnLeaseClosed(ctx, lease, types.LeaseClosed)
	ms.keepers.Market.OnOrderClosed(ctx, order)
	ms.keepers.Deployment.OnBidClosed(ctx, order.ID().GroupID())
	telemetry.IncrCounter(1.0, "akash.order_closed")

	return &types.MsgCloseBidResponse{}, nil
}

func (ms msgServer) WithdrawBid(goCtx context.Context, msg *types.MsgWithdrawBid) (*types.MsgWithdrawBidResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	_, found := ms.keepers.Market.GetBid(ctx, msg.BidID)
	if !found {
		return nil, types.ErrUnknownBid
	}

	if err := ms.keepers.Escrow.PaymentWithdraw(ctx,
		dtypes.EscrowAccountForDeployment(msg.BidID.DeploymentID()),
		types.EscrowPaymentForBid(msg.BidID),
	); err != nil {
		return &types.MsgWithdrawBidResponse{}, err
	}

	return &types.MsgWithdrawBidResponse{}, nil
}

func (ms msgServer) CreateLease(goCtx context.Context, msg *types.MsgCreateLease) (*types.MsgCreateLeaseResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	bid, found := ms.keepers.Market.GetBid(ctx, msg.BidID)
	if !found {
		return &types.MsgCreateLeaseResponse{}, types.ErrBidNotFound
	}

	if bid.State != types.BidOpen {
		// TODO: BidNotOpen
		return &types.MsgCreateLeaseResponse{}, types.ErrBidNotActive
	}

	order, found := ms.keepers.Market.GetOrder(ctx, msg.BidID.OrderID())
	if !found {
		return &types.MsgCreateLeaseResponse{}, types.ErrOrderNotFound
	}

	if order.State != types.OrderOpen {
		// TODO: OrderNotOpen
		return &types.MsgCreateLeaseResponse{}, types.ErrOrderNotFound
	}

	group, found := ms.keepers.Deployment.GetGroup(ctx, order.ID().GroupID())
	if !found {
		// TODO: not found
		return &types.MsgCreateLeaseResponse{}, types.ErrOrderNotFound
	}

	if group.State != dtypes.GroupOpen {
		// TODO: not found
		return &types.MsgCreateLeaseResponse{}, types.ErrOrderNotFound
	}

	owner, err := sdk.AccAddressFromBech32(msg.BidID.Provider)
	if err != nil {
		return &types.MsgCreateLeaseResponse{}, err
	}

	if err := ms.keepers.Escrow.PaymentCreate(ctx,
		dtypes.EscrowAccountForDeployment(msg.BidID.DeploymentID()),
		types.EscrowPaymentForBid(msg.BidID),
		owner,
		bid.Price); err != nil {
		return &types.MsgCreateLeaseResponse{}, err
	}

	ms.keepers.Market.CreateLease(ctx, bid)
	ms.keepers.Market.OnOrderMatched(ctx, order)
	ms.keepers.Market.OnBidMatched(ctx, bid)

	// close losing bids
	var lostbids []types.Bid
	ms.keepers.Market.WithBidsForOrder(ctx, msg.BidID.OrderID(), func(bid types.Bid) bool {
		if bid.ID().Equals(msg.BidID) {
			return false
		}
		if bid.State != types.BidOpen {
			return false
		}

		lostbids = append(lostbids, bid)
		return false
	})

	for _, bid := range lostbids {
		ms.keepers.Market.OnBidLost(ctx, bid)
		if err := ms.keepers.Escrow.AccountClose(ctx,
			types.EscrowAccountForBid(bid.ID())); err != nil {
			return &types.MsgCreateLeaseResponse{}, err
		}
	}

	return &types.MsgCreateLeaseResponse{}, nil
}

func (ms msgServer) CloseLease(goCtx context.Context, msg *types.MsgCloseLease) (*types.MsgCloseLeaseResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	order, found := ms.keepers.Market.GetOrder(ctx, msg.LeaseID.OrderID())
	if !found {
		return nil, types.ErrUnknownOrder
	}

	if order.State != types.OrderActive {
		return &types.MsgCloseLeaseResponse{}, types.ErrOrderClosed
	}

	lease, found := ms.keepers.Market.LeaseForOrder(ctx, order.ID())
	if !found {
		return &types.MsgCloseLeaseResponse{}, types.ErrNoLeaseForOrder
	}

	if lease.State != types.LeaseActive {
		return &types.MsgCloseLeaseResponse{}, types.ErrOrderClosed
	}

	if err := ms.keepers.Escrow.PaymentClose(ctx,
		dtypes.EscrowAccountForDeployment(lease.ID().DeploymentID()),
		types.EscrowPaymentForBid(lease.ID().BidID()),
	); err != nil {
		return &types.MsgCloseLeaseResponse{}, err
	}

	// closed by handlers.

	return &types.MsgCloseLeaseResponse{}, nil
}
