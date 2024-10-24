package simulation

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"pkg.akt.dev/go/node/market/v1beta5"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	keepers "pkg.akt.dev/node/x/market/handler"
)

func getOrdersWithState(ctx sdk.Context, ks keepers.Keepers, state v1beta5.Order_State) v1beta5.Orders {
	var orders v1beta5.Orders

	ks.Market.WithOrders(ctx, func(order v1beta5.Order) bool {
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
