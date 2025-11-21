package hooks

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
)

type Hooks interface {
	OnEscrowAccountClosed(ctx sdk.Context, obj etypes.Account) error
	OnEscrowPaymentClosed(ctx sdk.Context, obj etypes.Payment) error
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

func (h *hooks) OnEscrowAccountClosed(ctx sdk.Context, obj etypes.Account) error {
	id, err := dv1.DeploymentIDFromEscrowID(obj.ID)
	if err != nil {
		return err
	}

	deployment, found := h.dkeeper.GetDeployment(ctx, id)
	if !found {
		return nil
	}

	if deployment.State != dv1.DeploymentActive {
		return nil
	}
	err = h.dkeeper.CloseDeployment(ctx, deployment)
	if err != nil {
		return err
	}

	gstate := dtypes.GroupClosed
	if obj.State.State == etypes.StateOverdrawn {
		gstate = dtypes.GroupInsufficientFunds
	}

	for _, group := range h.dkeeper.GetGroups(ctx, deployment.ID) {
		if group.ValidateClosable() == nil {
			err = h.dkeeper.OnCloseGroup(ctx, group, gstate)
			if err != nil {
				return err
			}
			err = h.mkeeper.OnGroupClosed(ctx, group.ID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *hooks) OnEscrowPaymentClosed(ctx sdk.Context, obj etypes.Payment) error {
	id, err := mv1.LeaseIDFromPaymentID(obj.ID)
	if err != nil {
		return nil
	}

	bid, ok := h.mkeeper.GetBid(ctx, id.BidID())
	if !ok {
		return nil
	}

	if bid.State != mtypes.BidActive {
		return nil
	}

	order, ok := h.mkeeper.GetOrder(ctx, id.OrderID())
	if !ok {
		return mv1.ErrOrderNotFound
	}

	lease, ok := h.mkeeper.GetLease(ctx, id)
	if !ok {
		return mv1.ErrLeaseNotFound
	}

	err = h.mkeeper.OnOrderClosed(ctx, order)
	if err != nil {
		return err
	}
	err = h.mkeeper.OnBidClosed(ctx, bid)
	if err != nil {
		return err
	}

	if obj.State.State == etypes.StateOverdrawn {
		err = h.mkeeper.OnLeaseClosed(ctx, lease, mv1.LeaseInsufficientFunds, mv1.LeaseClosedReasonInsufficientFunds)
		if err != nil {
			return err
		}
	} else {
		err = h.mkeeper.OnLeaseClosed(ctx, lease, mv1.LeaseClosed, mv1.LeaseClosedReasonUnspecified)
		if err != nil {
			return err
		}
	}

	return nil
}
