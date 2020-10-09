package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/ovrclk/akash/x/market/types"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	Keeper
}

var _ types.QueryServer = Querier{}

// Orders returns orders based on filters
func (k Querier) Orders(c context.Context, req *types.QueryOrdersRequest) (*types.QueryOrdersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	stateVal := types.Order_State(types.Order_State_value[req.Filters.State])

	if req.Filters.State != "" && stateVal == types.OrderStateInvalid {
		return nil, status.Error(codes.InvalidArgument, "invalid state value")
	}

	var orders types.Orders
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(k.skey)
	orderStore := prefix.NewStore(store, orderPrefix)

	pageRes, err := sdkquery.FilteredPaginate(orderStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		var order types.Order

		err := k.cdc.UnmarshalBinaryBare(value, &order)
		if err != nil {
			return false, err
		}

		// filter orders with provided filters
		if req.Filters.Accept(order, stateVal) {
			if accumulate {
				orders = append(orders, order)
			}

			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryOrdersResponse{
		Orders:     orders,
		Pagination: pageRes,
	}, nil
}

// Order returns order details based on OrderID
func (k Querier) Order(c context.Context, req *types.QueryOrderRequest) (*types.QueryOrderResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if _, err := sdk.AccAddressFromBech32(req.ID.Owner); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner address")
	}

	ctx := sdk.UnwrapSDKContext(c)

	order, found := k.GetOrder(ctx, req.ID)
	if !found {
		return nil, types.ErrOrderNotFound
	}

	return &types.QueryOrderResponse{Order: order}, nil
}

// Bids returns bids based on filters
func (k Querier) Bids(c context.Context, req *types.QueryBidsRequest) (*types.QueryBidsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	stateVal := types.Bid_State(types.Bid_State_value[req.Filters.State])

	if req.Filters.State != "" && stateVal == types.BidStateInvalid {
		return nil, status.Error(codes.InvalidArgument, "invalid state value")
	}

	var bids types.Bids
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(k.skey)
	bidStore := prefix.NewStore(store, bidPrefix)

	pageRes, err := sdkquery.FilteredPaginate(bidStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		var bid types.Bid

		err := k.cdc.UnmarshalBinaryBare(value, &bid)
		if err != nil {
			return false, err
		}

		// filter bids with provided filters
		if req.Filters.Accept(bid, stateVal) {
			if accumulate {
				bids = append(bids, bid)
			}

			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryBidsResponse{
		Bids:       bids,
		Pagination: pageRes,
	}, nil
}

// Bid returns bid details based on BidID
func (k Querier) Bid(c context.Context, req *types.QueryBidRequest) (*types.QueryBidResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if _, err := sdk.AccAddressFromBech32(req.ID.Owner); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner address")
	}

	if _, err := sdk.AccAddressFromBech32(req.ID.Provider); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid provider address")
	}

	ctx := sdk.UnwrapSDKContext(c)

	bid, found := k.GetBid(ctx, req.ID)
	if !found {
		return nil, types.ErrBidNotFound
	}

	return &types.QueryBidResponse{Bid: bid}, nil
}

// Leases returns leases based on filters
func (k Querier) Leases(c context.Context, req *types.QueryLeasesRequest) (*types.QueryLeasesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	stateVal := types.Lease_State(types.Lease_State_value[req.Filters.State])

	if req.Filters.State != "" && stateVal == types.LeaseStateInvalid {
		return nil, status.Error(codes.InvalidArgument, "invalid state value")
	}

	var leases types.Leases
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(k.skey)
	leaseStore := prefix.NewStore(store, leasePrefix)

	pageRes, err := sdkquery.FilteredPaginate(leaseStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		var lease types.Lease

		err := k.cdc.UnmarshalBinaryBare(value, &lease)
		if err != nil {
			return false, err
		}

		// filter leases with provided filters
		if req.Filters.Accept(lease, stateVal) {
			if accumulate {
				leases = append(leases, lease)
			}

			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryLeasesResponse{
		Leases:     leases,
		Pagination: pageRes,
	}, nil
}

// Lease returns lease details based on LeaseID
func (k Querier) Lease(c context.Context, req *types.QueryLeaseRequest) (*types.QueryLeaseResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if _, err := sdk.AccAddressFromBech32(req.ID.Owner); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner address")
	}

	if _, err := sdk.AccAddressFromBech32(req.ID.Provider); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid provider address")
	}

	ctx := sdk.UnwrapSDKContext(c)

	lease, found := k.GetLease(ctx, req.ID)
	if !found {
		return nil, types.ErrLeaseNotFound
	}

	return &types.QueryLeaseResponse{Lease: lease}, nil
}
