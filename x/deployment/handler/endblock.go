package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

// OnEndBlock create order and update order state for each deployment
// Executed at the end of block
func OnEndBlock(ctx sdk.Context, keeper keeper.Keeper, mkeeper MarketKeeper) {

	// create orders as necessary for Active Deployment
	keeper.WithDeploymentsActive(ctx, func(d types.Deployment) bool {
		for _, group := range keeper.GetGroups(ctx, d.ID()) {

			// open groups only
			if err := group.ValidateOrderable(); err != nil {
				continue
			}

			// create order.
			if _, err := mkeeper.CreateOrder(ctx, group.ID(), group.GroupSpec); err != nil {
				ctx.Logger().With("group", group.ID(), "error", err).Error("creating order")
				continue
			}

			// set state to ordered
			keeper.OnOrderCreated(ctx, group)
		}
		return false
	})
}
