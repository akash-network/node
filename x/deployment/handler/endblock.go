package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

// OnEndBlock create order and update order state for each open Group.
// Executed at the end of block
func OnEndBlock(ctx sdk.Context, keeper keeper.Keeper, mkeeper MarketKeeper) {
	// For each open group; create an order and update the Group's state.
	keeper.WithOpenGroups(ctx, func(group types.Group) bool {
		// create order.
		if _, err := mkeeper.CreateOrder(ctx, group.ID(), group.GroupSpec); err != nil {
			ctx.Logger().With("group", group.ID(), "error", err).Error("creating order")
		}

		// set state to ordered
		keeper.OnOrderCreated(ctx, group)
		return false
	})
}
