package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

func NewQuerier(keeper keeper.Keeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, sdk.Error) {
		switch path[0] {
		case ordersPath:
			return queryOrders(ctx, path[1:], req, keeper)
		case bidsPath:
			return queryBids(ctx, path[1:], req, keeper)
		case leasesPath:
			return queryLeases(ctx, path[1:], req, keeper)
		}
		return []byte{}, sdk.ErrUnknownRequest("unknown query path")
	}
}

func queryOrders(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, sdk.Error) {
	var values Orders
	keeper.WithOrders(ctx, func(obj types.Order) bool {
		values = append(values, Order(obj))
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryBids(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, sdk.Error) {
	var values Bids
	keeper.WithBids(ctx, func(obj types.Bid) bool {
		values = append(values, Bid(obj))
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryLeases(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, sdk.Error) {
	var values Leases
	keeper.WithLeases(ctx, func(obj types.Lease) bool {
		values = append(values, Lease(obj))
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}
