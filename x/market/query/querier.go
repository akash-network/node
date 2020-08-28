package query

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// NewQuerier creates and returns a new market querier instance
func NewQuerier(keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
		switch path[0] {
		case ordersPath:
			return queryOrders(ctx, path[1:], req, keeper, legacyQuerierCdc)
		case orderPath:
			return queryOrder(ctx, path[1:], req, keeper, legacyQuerierCdc)
		case bidsPath:
			return queryBids(ctx, path[1:], req, keeper, legacyQuerierCdc)
		case bidPath:
			return queryBid(ctx, path[1:], req, keeper, legacyQuerierCdc)
		case leasesPath:
			return queryLeases(ctx, path[1:], req, keeper, legacyQuerierCdc)
		case leasePath:
			return queryLease(ctx, path[1:], req, keeper, legacyQuerierCdc)
		}
		return []byte{}, sdkerrors.ErrUnknownRequest
	}
}

func queryOrders(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper,
	legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {
	// isValidState denotes whether given state flag is valid or not
	filters, isValidState, err := parseOrderFiltersPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	var values Orders
	keeper.WithOrders(ctx, func(obj types.Order) bool {
		if filters.Accept(obj, isValidState) {
			values = append(values, Order(obj))
		}

		return false
	})
	return sdkutil.RenderQueryResponse(legacyQuerierCdc, values)
}

func queryOrder(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper,
	legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {

	id, err := parseOrderPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	order, ok := keeper.GetOrder(ctx, id)
	if !ok {
		return nil, types.ErrOrderNotFound
	}

	value := Order(order)

	return sdkutil.RenderQueryResponse(legacyQuerierCdc, value)
}

func queryBids(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper,
	legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {
	// isValidState denotes whether given state flag is valid or not
	filters, isValidState, err := parseBidFiltersPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}
	var values Bids
	keeper.WithBids(ctx, func(obj types.Bid) bool {
		if filters.Accept(obj, isValidState) {
			values = append(values, Bid(obj))
		}
		return false
	})
	return sdkutil.RenderQueryResponse(legacyQuerierCdc, values)
}

func queryBid(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper,
	legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {

	id, err := parseBidPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	bid, ok := keeper.GetBid(ctx, id)
	if !ok {
		return nil, types.ErrBidNotFound
	}

	value := Bid(bid)

	return sdkutil.RenderQueryResponse(legacyQuerierCdc, value)
}

func queryLeases(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper,
	legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {
	// isValidState denotes whether given state flag is valid or not
	filters, isValidState, err := parseLeaseFiltersPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}
	var values Leases
	keeper.WithLeases(ctx, func(obj types.Lease) bool {
		if filters.Accept(obj, isValidState) {
			values = append(values, Lease(obj))
		}
		return false
	})
	return sdkutil.RenderQueryResponse(legacyQuerierCdc, values)
}

func queryLease(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {

	id, err := ParseLeasePath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	lease, ok := keeper.GetLease(ctx, id)
	if !ok {
		return nil, types.ErrLeaseNotFound
	}

	value := Lease(lease)

	return sdkutil.RenderQueryResponse(legacyQuerierCdc, value)
}
