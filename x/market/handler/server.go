package handler

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	atypes "pkg.akt.dev/go/node/audit/v1"
	dbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

type msgServer struct {
	keepers Keepers
}

// NewServer returns an implementation of the market MsgServer interface
// for the provided Keeper.
func NewServer(k Keepers) mtypes.MsgServer {
	return &msgServer{keepers: k}
}

var _ mtypes.MsgServer = msgServer{}

func (ms msgServer) CreateBid(goCtx context.Context, msg *mtypes.MsgCreateBid) (*mtypes.MsgCreateBidResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params := ms.keepers.Market.GetParams(ctx)

	minDeposit := params.BidMinDeposit
	if msg.Deposit.Amount.Denom != minDeposit.Denom {
		return nil, fmt.Errorf("%w: mininum:%v received:%v", mv1.ErrInvalidDeposit, minDeposit, msg.Deposit)
	}
	if minDeposit.Amount.GT(msg.Deposit.Amount.Amount) {
		return nil, fmt.Errorf("%w: mininum:%v received:%v", mv1.ErrInvalidDeposit, minDeposit, msg.Deposit)
	}

	if ms.keepers.Market.BidCountForOrder(ctx, msg.ID.OrderID()) > params.OrderMaxBids {
		return nil, fmt.Errorf("%w: too many existing bids (%v)", mv1.ErrInvalidBid, params.OrderMaxBids)
	}

	if msg.ID.BSeq != 0 {
		return nil, mv1.ErrInvalidBid
	}

	order, found := ms.keepers.Market.GetOrder(ctx, msg.ID.OrderID())
	if !found {
		return nil, mv1.ErrOrderNotFound
	}

	if err := order.ValidateCanBid(); err != nil {
		return nil, err
	}

	if !msg.Price.IsValid() {
		return nil, mv1.ErrBidInvalidPrice
	}

	if order.Price().IsLT(msg.Price) {
		return nil, mv1.ErrBidInvalidPrice
	}

	if !msg.ResourcesOffer.MatchGSpec(order.Spec) {
		return nil, mv1.ErrCapabilitiesMismatch
	}

	provider, err := sdk.AccAddressFromBech32(msg.ID.Provider)
	if err != nil {
		return nil, mv1.ErrEmptyProvider
	}

	var prov ptypes.Provider
	if prov, found = ms.keepers.Provider.Get(ctx, provider); !found {
		return nil, mv1.ErrUnknownProvider
	}

	provAttr, _ := ms.keepers.Audit.GetProviderAttributes(ctx, provider)

	provAttr = append([]atypes.AuditedProvider{{
		Owner:      msg.ID.Provider,
		Attributes: prov.Attributes,
	}}, provAttr...)

	if !order.MatchRequirements(provAttr) {
		return nil, mv1.ErrAttributeMismatch
	}

	if !order.MatchResourcesRequirements(prov.Attributes) {
		return nil, mv1.ErrCapabilitiesMismatch
	}

	deposits, err := ms.keepers.Escrow.AuthorizeDeposits(ctx, msg)
	if err != nil {
		return nil, err
	}

	bid, err := ms.keepers.Market.CreateBid(ctx, msg.ID, msg.Price, msg.ResourcesOffer)
	if err != nil {
		return nil, err
	}

	// create an escrow account for this bid
	err = ms.keepers.Escrow.AccountCreate(ctx, bid.ID.ToEscrowAccountID(), provider, deposits)
	if err != nil {
		return &mtypes.MsgCreateBidResponse{}, err
	}

	telemetry.IncrCounter(1.0, "akash.bids")
	return &mtypes.MsgCreateBidResponse{}, nil
}

func (ms msgServer) CloseBid(goCtx context.Context, msg *mtypes.MsgCloseBid) (*mtypes.MsgCloseBidResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	bid, found := ms.keepers.Market.GetBid(ctx, msg.ID)
	if !found {
		return nil, mv1.ErrUnknownBid
	}

	order, found := ms.keepers.Market.GetOrder(ctx, msg.ID.OrderID())
	if !found {
		return nil, mv1.ErrUnknownOrderForBid
	}

	if bid.State == mtypes.BidOpen {
		_ = ms.keepers.Market.OnBidClosed(ctx, bid)
		return &mtypes.MsgCloseBidResponse{}, nil
	}

	lease, found := ms.keepers.Market.GetLease(ctx, mv1.LeaseID(msg.ID))
	if !found {
		return nil, mv1.ErrUnknownLeaseForBid
	}

	if lease.State != mv1.LeaseActive {
		return nil, mv1.ErrLeaseNotActive
	}

	if bid.State != mtypes.BidActive {
		return nil, mv1.ErrBidNotActive
	}

	if err := ms.keepers.Deployment.OnBidClosed(ctx, order.ID.GroupID()); err != nil {
		return nil, err
	}

	_ = ms.keepers.Market.OnLeaseClosed(ctx, lease, mv1.LeaseClosed, msg.Reason)
	_ = ms.keepers.Market.OnBidClosed(ctx, bid)
	_ = ms.keepers.Market.OnOrderClosed(ctx, order)

	_ = ms.keepers.Escrow.PaymentClose(ctx, lease.ID.ToEscrowPaymentID())

	telemetry.IncrCounter(1.0, "akash.order_closed")

	return &mtypes.MsgCloseBidResponse{}, nil
}

func (ms msgServer) WithdrawLease(goCtx context.Context, msg *mtypes.MsgWithdrawLease) (*mtypes.MsgWithdrawLeaseResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	_, found := ms.keepers.Market.GetLease(ctx, msg.ID)
	if !found {
		return nil, mv1.ErrUnknownLease
	}

	if err := ms.keepers.Escrow.PaymentWithdraw(ctx, msg.ID.ToEscrowPaymentID()); err != nil {
		return &mtypes.MsgWithdrawLeaseResponse{}, err
	}

	return &mtypes.MsgWithdrawLeaseResponse{}, nil
}

func (ms msgServer) CreateLease(goCtx context.Context, msg *mtypes.MsgCreateLease) (*mtypes.MsgCreateLeaseResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	bid, found := ms.keepers.Market.GetBid(ctx, msg.BidID)
	if !found {
		return &mtypes.MsgCreateLeaseResponse{}, mv1.ErrBidNotFound
	}

	if bid.State != mtypes.BidOpen {
		return &mtypes.MsgCreateLeaseResponse{}, mv1.ErrBidNotOpen
	}

	order, found := ms.keepers.Market.GetOrder(ctx, msg.BidID.OrderID())
	if !found {
		return &mtypes.MsgCreateLeaseResponse{}, mv1.ErrOrderNotFound
	}

	if order.State != mtypes.OrderOpen {
		return &mtypes.MsgCreateLeaseResponse{}, mv1.ErrOrderNotOpen
	}

	group, found := ms.keepers.Deployment.GetGroup(ctx, order.ID.GroupID())
	if !found {
		return &mtypes.MsgCreateLeaseResponse{}, mv1.ErrGroupNotFound
	}

	if group.State != dbeta.GroupOpen {
		return &mtypes.MsgCreateLeaseResponse{}, mv1.ErrGroupNotOpen
	}

	provider, err := sdk.AccAddressFromBech32(msg.BidID.Provider)
	if err != nil {
		return &mtypes.MsgCreateLeaseResponse{}, err
	}

	// Convert bid price from uakt to uact if needed (account funds are in uact after BME conversion)
	// Swap rate: 1 uakt = 3 uact (based on oracle prices: AKT=$3, ACT=$1)
	paymentRate := bid.Price

	err = ms.keepers.Escrow.PaymentCreate(ctx, msg.BidID.LeaseID().ToEscrowPaymentID(), provider, paymentRate)
	if err != nil {
		return &mtypes.MsgCreateLeaseResponse{}, err
	}

	err = ms.keepers.Market.CreateLease(ctx, bid)
	if err != nil {
		return &mtypes.MsgCreateLeaseResponse{}, err
	}

	ms.keepers.Market.OnOrderMatched(ctx, order)
	ms.keepers.Market.OnBidMatched(ctx, bid)

	// close losing bids
	ms.keepers.Market.WithBidsForOrder(ctx, msg.BidID.OrderID(), mtypes.BidOpen, func(cbid mtypes.Bid) bool {
		ms.keepers.Market.OnBidLost(ctx, cbid)

		if err = ms.keepers.Escrow.AccountClose(ctx, cbid.ID.ToEscrowAccountID()); err != nil {
			return true
		}
		return false
	})

	return &mtypes.MsgCreateLeaseResponse{}, nil
}

func (ms msgServer) CloseLease(goCtx context.Context, msg *mtypes.MsgCloseLease) (*mtypes.MsgCloseLeaseResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	order, found := ms.keepers.Market.GetOrder(ctx, msg.ID.OrderID())
	if !found {
		return nil, mv1.ErrOrderNotFound
	}

	if order.State != mtypes.OrderActive {
		return &mtypes.MsgCloseLeaseResponse{}, mv1.ErrOrderClosed
	}

	bid, found := ms.keepers.Market.GetBid(ctx, msg.ID.BidID())
	if !found {
		return &mtypes.MsgCloseLeaseResponse{}, mv1.ErrBidNotFound
	}
	if bid.State != mtypes.BidActive {
		return &mtypes.MsgCloseLeaseResponse{}, mv1.ErrBidNotActive
	}

	lease, found := ms.keepers.Market.GetLease(ctx, msg.ID)
	if !found {
		return &mtypes.MsgCloseLeaseResponse{}, mv1.ErrLeaseNotFound
	}
	if lease.State != mv1.LeaseActive {
		return &mtypes.MsgCloseLeaseResponse{}, mv1.ErrOrderClosed
	}

	_ = ms.keepers.Market.OnLeaseClosed(ctx, lease, mv1.LeaseClosed, mv1.LeaseClosedReasonOwner)
	_ = ms.keepers.Market.OnBidClosed(ctx, bid)
	_ = ms.keepers.Market.OnOrderClosed(ctx, order)

	err := ms.keepers.Escrow.PaymentClose(ctx, lease.ID.ToEscrowPaymentID())
	if err != nil {
		return &mtypes.MsgCloseLeaseResponse{}, err
	}

	group, err := ms.keepers.Deployment.OnLeaseClosed(ctx, msg.ID.GroupID())
	if err != nil {
		return &mtypes.MsgCloseLeaseResponse{}, err
	}

	if group.State != dbeta.GroupOpen {
		return &mtypes.MsgCloseLeaseResponse{}, nil
	}

	if _, err := ms.keepers.Market.CreateOrder(ctx, group.ID, group.GroupSpec); err != nil {
		return &mtypes.MsgCloseLeaseResponse{}, err
	}

	return &mtypes.MsgCloseLeaseResponse{}, nil
}

func (ms msgServer) UpdateParams(goCtx context.Context, req *mtypes.MsgUpdateParams) (*mtypes.MsgUpdateParamsResponse, error) {
	if ms.keepers.Market.GetAuthority() != req.Authority {
		return nil, govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", ms.keepers.Market.GetAuthority(), req.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := ms.keepers.Market.SetParams(ctx, req.Params); err != nil {
		return nil, err
	}

	return &mtypes.MsgUpdateParamsResponse{}, nil
}
