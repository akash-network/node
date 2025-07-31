package hooks

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
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

	if deployment.State != dv1.DeploymentActive {
		return
	}
	_ = h.dkeeper.CloseDeployment(ctx, deployment)

	gstate := dtypes.GroupClosed
	if obj.State == etypes.AccountOverdrawn {
		gstate = dtypes.GroupInsufficientFunds
	}

	for _, group := range h.dkeeper.GetGroups(ctx, deployment.ID) {
		if group.ValidateClosable() == nil {
			_ = h.dkeeper.OnCloseGroup(ctx, group, gstate)
			_ = h.mkeeper.OnGroupClosed(ctx, group.ID)
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

	_ = h.mkeeper.OnOrderClosed(ctx, order)
	_ = h.mkeeper.OnBidClosed(ctx, bid)

	if obj.State == etypes.PaymentOverdrawn {
		_ = h.mkeeper.OnLeaseClosed(ctx, lease, mv1.LeaseInsufficientFunds)
	} else {
		_ = h.mkeeper.OnLeaseClosed(ctx, lease, mv1.LeaseClosed)
	}
}
