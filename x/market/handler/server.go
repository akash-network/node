package handler

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"

	atypes "github.com/akash-network/akash-api/go/node/audit/v1beta3"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/market/v1beta3"

	ptypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"
)

type msgServer struct {
	keepers Keepers
}

// NewServer returns an implementation of the market MsgServer interface
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
		return nil, fmt.Errorf("%w: mininum:%v received:%v", types.ErrInvalidDeposit, minDeposit, msg.Deposit)
	}
	if minDeposit.Amount.GT(msg.Deposit.Amount) {
		return nil, fmt.Errorf("%w: mininum:%v received:%v", types.ErrInvalidDeposit, minDeposit, msg.Deposit)
	}

	if ms.keepers.Market.BidCountForOrder(ctx, msg.Order) > params.OrderMaxBids {
		return nil, fmt.Errorf("%w: too many existing bids (%v)", types.ErrInvalidBid, params.OrderMaxBids)
	}

	order, found := ms.keepers.Market.GetOrder(ctx, msg.Order)
	if !found {
		return nil, types.ErrOrderNotFound
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
		return nil, types.ErrUnknownProvider
	}

	provAttr, _ := ms.keepers.Audit.GetProviderAttributes(ctx, provider)

	provAttr = append([]atypes.Provider{{
		Owner:      msg.Provider,
		Attributes: prov.Attributes,
	}}, provAttr...)

	if !order.MatchRequirements(provAttr) {
		return nil, types.ErrAttributeMismatch
	}

	if !order.MatchResourcesRequirements(prov.Attributes) {
		return nil, types.ErrCapabilitiesMismatch
	}

	bid, err := ms.keepers.Market.CreateBid(ctx, msg.Order, provider, msg.Price)
	if err != nil {
		return nil, err
	}

	// create escrow account for this bid
	if err := ms.keepers.Escrow.AccountCreate(ctx,
		types.EscrowAccountForBid(bid.ID()),
		provider,
		provider, // bids currently don't support deposits by non-owners
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

	if err := ms.keepers.Deployment.OnBidClosed(ctx, order.ID().GroupID()); err != nil {
		return nil, err
	}

	ms.keepers.Market.OnLeaseClosed(ctx, lease, types.LeaseClosed)
	ms.keepers.Market.OnBidClosed(ctx, bid)
	ms.keepers.Market.OnOrderClosed(ctx, order)

	ms.keepers.Escrow.PaymentClose(ctx,
		dtypes.EscrowAccountForDeployment(lease.ID().DeploymentID()),
		types.EscrowPaymentForLease(lease.ID()))

	telemetry.IncrCounter(1.0, "akash.order_closed")

	return &types.MsgCloseBidResponse{}, nil
}

func (ms msgServer) WithdrawLease(goCtx context.Context, msg *types.MsgWithdrawLease) (*types.MsgWithdrawLeaseResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	_, found := ms.keepers.Market.GetLease(ctx, msg.LeaseID)
	if !found {
		return nil, types.ErrUnknownLease
	}

	if err := ms.keepers.Escrow.PaymentWithdraw(ctx,
		dtypes.EscrowAccountForDeployment(msg.LeaseID.DeploymentID()),
		types.EscrowPaymentForLease(msg.LeaseID),
	); err != nil {
		return &types.MsgWithdrawLeaseResponse{}, err
	}

	return &types.MsgWithdrawLeaseResponse{}, nil
}

func (ms msgServer) CreateLease(goCtx context.Context, msg *types.MsgCreateLease) (*types.MsgCreateLeaseResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	bid, found := ms.keepers.Market.GetBid(ctx, msg.BidID)
	if !found {
		return &types.MsgCreateLeaseResponse{}, types.ErrBidNotFound
	}

	if bid.State != types.BidOpen {
		return &types.MsgCreateLeaseResponse{}, types.ErrBidNotOpen
	}

	order, found := ms.keepers.Market.GetOrder(ctx, msg.BidID.OrderID())
	if !found {
		return &types.MsgCreateLeaseResponse{}, types.ErrOrderNotFound
	}

	if order.State != types.OrderOpen {
		return &types.MsgCreateLeaseResponse{}, types.ErrOrderNotOpen
	}

	group, found := ms.keepers.Deployment.GetGroup(ctx, order.ID().GroupID())
	if !found {
		return &types.MsgCreateLeaseResponse{}, types.ErrGroupNotFound
	}

	if group.State != dtypes.GroupOpen {
		return &types.MsgCreateLeaseResponse{}, types.ErrGroupNotOpen
	}

	owner, err := sdk.AccAddressFromBech32(msg.BidID.Provider)
	if err != nil {
		return &types.MsgCreateLeaseResponse{}, err
	}

	if err := ms.keepers.Escrow.PaymentCreate(ctx,
		dtypes.EscrowAccountForDeployment(msg.BidID.DeploymentID()),
		types.EscrowPaymentForLease(msg.BidID.LeaseID()),
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
		return nil, types.ErrOrderNotFound
	}

	if order.State != types.OrderActive {
		return &types.MsgCloseLeaseResponse{}, types.ErrOrderClosed
	}

	bid, found := ms.keepers.Market.GetBid(ctx, msg.LeaseID.BidID())
	if !found {
		return &types.MsgCloseLeaseResponse{}, types.ErrBidNotFound
	}
	if bid.State != types.BidActive {
		return &types.MsgCloseLeaseResponse{}, types.ErrBidNotActive
	}

	lease, found := ms.keepers.Market.GetLease(ctx, msg.LeaseID)
	if !found {
		return &types.MsgCloseLeaseResponse{}, types.ErrLeaseNotFound
	}
	if lease.State != types.LeaseActive {
		return &types.MsgCloseLeaseResponse{}, types.ErrOrderClosed
	}

	ms.keepers.Market.OnLeaseClosed(ctx, lease, types.LeaseClosed)
	ms.keepers.Market.OnBidClosed(ctx, bid)
	ms.keepers.Market.OnOrderClosed(ctx, order)

	if err := ms.keepers.Escrow.PaymentClose(ctx,
		dtypes.EscrowAccountForDeployment(lease.ID().DeploymentID()),
		types.EscrowPaymentForLease(lease.ID()),
	); err != nil {
		return &types.MsgCloseLeaseResponse{}, err
	}

	group, err := ms.keepers.Deployment.OnLeaseClosed(ctx, msg.LeaseID.GroupID())
	if err != nil {
		return &types.MsgCloseLeaseResponse{}, err
	}

	if group.State != dtypes.GroupOpen {
		return &types.MsgCloseLeaseResponse{}, nil
	}
	if _, err := ms.keepers.Market.CreateOrder(ctx, group.ID(), group.GroupSpec); err != nil {
		return &types.MsgCloseLeaseResponse{}, err
	}
	return &types.MsgCloseLeaseResponse{}, nil

}
