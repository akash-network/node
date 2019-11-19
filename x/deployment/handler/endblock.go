package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

func OnEndBlock(ctx sdk.Context, keeper keeper.Keeper, mkeeper MarketKeeper) {

	// create orders as necessary
	keeper.WithDeployments(ctx, func(d types.Deployment) bool {

		// active deployments only
		if d.State != types.DeploymentActive {
			return false
		}

		for _, group := range keeper.GetGroups(ctx, d.ID()) {

			// open groups only
			if err := group.ValidateOrderable(); err != nil {
				continue
			}

			// TODO: check for active order.

			// create order.
			mkeeper.CreateOrder(ctx, group.ID(), group.GroupSpec)

			// set state to ordered
			keeper.OnOrderCreated(ctx, group)
		}

		return false
	})

}
