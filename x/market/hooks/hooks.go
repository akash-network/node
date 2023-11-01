package hooks

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	etypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"

	mtypes "github.com/akash-network/akash-api/go/node/market/v1beta4"
)

type Hooks interface {
	OnEscrowAccountClosed(ctx sdk.Context, obj etypes.Account)
	OnEscrowPaymentClosed(ctx sdk.Context, obj etypes.FractionalPayment)
}

type hooks struct {
	dkeeper DeploymentKeeper
	mkeeper MarketKeeper
}

func New(dkeeper DeploymentKeeper, mkeeper MarketKeeper) Hooks {
	return &hooks{
		dkeeper: dkeeper,
		mkeeper: mkeeper,
	}
}

func (h *hooks) OnEscrowAccountClosed(ctx sdk.Context, obj etypes.Account) {
	id, found := dtypes.DeploymentIDFromEscrowAccount(obj.ID)
	if !found {
		return
	}

	deployment, found := h.dkeeper.GetDeployment(ctx, id)
	if !found {
		return
	}

	if deployment.State != dtypes.DeploymentActive {
		return
	}
	h.dkeeper.CloseDeployment(ctx, deployment)

	gstate := dtypes.GroupClosed
	if obj.State == etypes.AccountOverdrawn {
		gstate = dtypes.GroupInsufficientFunds
	}

	for _, group := range h.dkeeper.GetGroups(ctx, deployment.ID()) {
		if group.ValidateClosable() == nil {
			_ = h.dkeeper.OnCloseGroup(ctx, group, gstate)
			h.mkeeper.OnGroupClosed(ctx, group.ID())
		}
	}
}

func (h *hooks) OnEscrowPaymentClosed(ctx sdk.Context, obj etypes.FractionalPayment) {
	id, ok := mtypes.LeaseIDFromEscrowAccount(obj.AccountID, obj.PaymentID)
	if !ok {
		return
	}

	bid, ok := h.mkeeper.GetBid(ctx, id.BidID())
	if !ok {
		return
	}

	if bid.State != mtypes.BidActive {
		return
	}

	order, ok := h.mkeeper.GetOrder(ctx, id.OrderID())
	if !ok {
		return
	}

	lease, ok := h.mkeeper.GetLease(ctx, id)
	if !ok {
		return
	}

	h.mkeeper.OnOrderClosed(ctx, order)
	h.mkeeper.OnBidClosed(ctx, bid)

	if obj.State == etypes.PaymentOverdrawn {
		h.mkeeper.OnLeaseClosed(ctx, lease, mtypes.LeaseInsufficientFunds)
	} else {
		h.mkeeper.OnLeaseClosed(ctx, lease, mtypes.LeaseClosed)
	}
}
