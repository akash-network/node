package hooks

import (
	dtypes "github.com/akash-network/node/x/deployment/types/v1beta2"
	mtypes "github.com/akash-network/node/x/market/types/v1beta2"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type DeploymentKeeper interface {
	GetDeployment(ctx sdk.Context, id dtypes.DeploymentID) (dtypes.Deployment, bool)
	GetGroups(ctx sdk.Context, id dtypes.DeploymentID) []dtypes.Group
	CloseDeployment(ctx sdk.Context, deployment dtypes.Deployment)
	OnCloseGroup(ctx sdk.Context, group dtypes.Group, state dtypes.Group_State) error
}

type MarketKeeper interface {
	GetOrder(ctx sdk.Context, id mtypes.OrderID) (mtypes.Order, bool)
	GetBid(ctx sdk.Context, id mtypes.BidID) (mtypes.Bid, bool)
	GetLease(ctx sdk.Context, id mtypes.LeaseID) (mtypes.Lease, bool)
	OnGroupClosed(ctx sdk.Context, id dtypes.GroupID)
	OnOrderClosed(ctx sdk.Context, order mtypes.Order)
	OnBidClosed(ctx sdk.Context, bid mtypes.Bid)
	OnLeaseClosed(ctx sdk.Context, lease mtypes.Lease, state mtypes.Lease_State)
}
