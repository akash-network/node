package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ovrclk/akash/sdkutil"
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
		case filterordersPath:
			return queryFilterOrders(ctx, path[1:], req, keeper)
		case orderPath:
			return queryOrder(ctx, path[1:], req, keeper)
		case bidsPath:
			return queryBids(ctx, path[1:], req, keeper)
		case filterbidsPath:
			return queryFilterBids(ctx, path[1:], req, keeper)
		case bidPath:
			return queryBid(ctx, path[1:], req, keeper)
		case leasesPath:
			return queryLeases(ctx, path[1:], req, keeper)
		case filterleasesPath:
			return queryFilterLeases(ctx, path[1:], req, keeper)
		case leasePath:
			return queryLease(ctx, path[1:], req, keeper)
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
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryFilterOrders(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	filter, err := parseOrderFiltersPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	var values Orders
	keeper.WithOrders(ctx, func(obj types.Order) bool {
		// Filtering orders based on flags
		if filter.Owner.Empty() {
			if obj.State == filter.State {
				values = append(values, Order(obj))
			}
		} else if filter.State == 100 {
			if obj.OrderID.Owner.Equals(filter.Owner) {
				values = append(values, Order(obj))
			}
		} else {
			if obj.OrderID.Owner.Equals(filter.Owner) && obj.State == filter.State {
				values = append(values, Order(obj))
			}
		}
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryOrder(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

	id, err := parseOrderPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	order, ok := keeper.GetOrder(ctx, id)
	if !ok {
		return nil, types.ErrOrderNotFound
	}

	value := Order(order)

	return sdkutil.RenderQueryResponse(keeper.Codec(), value)
}

func queryBids(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	var values Bids
	keeper.WithBids(ctx, func(obj types.Bid) bool {
		values = append(values, Bid(obj))
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryFilterBids(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	filter, err := parseBidFiltersPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	var values Bids
	keeper.WithBids(ctx, func(obj types.Bid) bool {
		// Filtering bids based on flags
		if filter.Owner.Empty() {
			if obj.State == filter.State {
				values = append(values, Bid(obj))
			}
		} else if filter.State == 100 {
			if obj.BidID.Owner.Equals(filter.Owner) {
				values = append(values, Bid(obj))
			}
		} else {
			if obj.BidID.Owner.Equals(filter.Owner) && obj.State == filter.State {
				values = append(values, Bid(obj))
			}
		}
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryBid(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

	id, err := parseBidPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	bid, ok := keeper.GetBid(ctx, id)
	if !ok {
		return nil, types.ErrBidNotFound
	}

	value := Bid(bid)

	return sdkutil.RenderQueryResponse(keeper.Codec(), value)
}

func queryLeases(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	var values Leases
	keeper.WithLeases(ctx, func(obj types.Lease) bool {
		values = append(values, Lease(obj))
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryFilterLeases(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	filter, err := parseLeaseFiltersPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	var values Leases
	keeper.WithLeases(ctx, func(obj types.Lease) bool {
		// Filtering deployments based on flags
		if filter.Owner.Empty() {
			if obj.State == filter.State {
				values = append(values, Lease(obj))
			}
		} else if filter.State == 100 {
			if obj.LeaseID.Owner.Equals(filter.Owner) {
				values = append(values, Lease(obj))
			}
		} else {
			if obj.LeaseID.Owner.Equals(filter.Owner) && obj.State == filter.State {
				values = append(values, Lease(obj))
			}
		}
		return false
	})
	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryLease(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

	id, err := parseLeasePath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	lease, ok := keeper.GetLease(ctx, id)
	if !ok {
		return nil, types.ErrLeaseNotFound
	}

	value := Lease(lease)

	return sdkutil.RenderQueryResponse(keeper.Codec(), value)
}
