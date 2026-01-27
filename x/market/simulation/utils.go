package simulation

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	keepers "pkg.akt.dev/node/v2/x/market/handler"
)

func getOrdersWithState(ctx sdk.Context, ks keepers.Keepers, state mtypes.Order_State) mtypes.Orders {
	var orders mtypes.Orders

	ks.Market.WithOrders(ctx, func(order mtypes.Order) bool {
		if order.State == state {
			orders = append(orders, order)
		}

		return false
	})

	return orders
}

func getProviders(ctx sdk.Context, ks keepers.Keepers) []ptypes.Provider {
	var providers []ptypes.Provider

	ks.Provider.WithProviders(ctx, func(provider ptypes.Provider) bool {
		providers = append(providers, provider)

		return false
	})

	return providers
}
