package query

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// NewQuerier creates and returns a new market querier instance
func NewQuerier(keeper keeper.Keeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
		switch path[0] {
		case ordersPath:
			return queryOrders(ctx, path[1:], req, keeper)
		case bidsPath:
			return queryBids(ctx, path[1:], req, keeper)
		case leasesPath:
			return queryLeases(ctx, path[1:], req, keeper)
		}
		return []byte{}, sdkerrors.ErrUnknownRequest
	}
}

func queryOrders(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	var values Orders
	keeper.WithOrders(ctx, func(obj types.Order) bool {
		values = append(values, Order(obj))
		return false
	})
	return codec.MarshalJSONIndent(keeper.Codec(), values)
}

func queryBids(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	var values Bids
	keeper.WithBids(ctx, func(obj types.Bid) bool {
		values = append(values, Bid(obj))
		return false
	})
	return codec.MarshalJSONIndent(keeper.Codec(), values)
}

func queryLeases(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	var values Leases
	keeper.WithLeases(ctx, func(obj types.Lease) bool {
		values = append(values, Lease(obj))
		return false
	})
	return codec.MarshalJSONIndent(keeper.Codec(), values)
}
