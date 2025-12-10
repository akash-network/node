package hooks

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta5"
	mtypes "pkg.akt.dev/go/node/market/v2beta1"
)

type DeploymentKeeper interface {
	GetDeployment(ctx sdk.Context, id dv1.DeploymentID) (dv1.Deployment, bool)
	GetGroups(ctx sdk.Context, id dv1.DeploymentID) dtypes.Groups
	CloseDeployment(ctx sdk.Context, deployment dv1.Deployment) error
	OnCloseGroup(ctx sdk.Context, group dtypes.Group, state dtypes.Group_State) error
}

type MarketKeeper interface {
	GetOrder(ctx sdk.Context, id mtypes.OrderID) (mtypes.Order, bool)
	GetBid(ctx sdk.Context, id mtypes.BidID) (mtypes.Bid, bool)
	GetLease(ctx sdk.Context, id mtypes.LeaseID) (mtypes.Lease, bool)
	OnGroupClosed(ctx sdk.Context, id dv1.GroupID) error
	OnOrderClosed(ctx sdk.Context, order mtypes.Order) error
	OnBidClosed(ctx sdk.Context, bid mtypes.Bid) error
	OnLeaseClosed(ctx sdk.Context, lease mtypes.Lease, state mtypes.Lease_State, reason mtypes.LeaseClosedReason) error
}
