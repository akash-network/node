package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	keys "github.com/akash-network/node/x/market/keeper/keys/v1beta4"
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
	searchPrefix, err := keys.OrderPrefixFromFilter(req.Filters)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	orderStore := prefix.NewStore(store, searchPrefix)

	pageRes, err := sdkquery.FilteredPaginate(orderStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		var order types.Order

		err := k.cdc.Unmarshal(value, &order)
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

	var bids []types.QueryBidResponse
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(k.skey)
	searchPrefix, err := keys.BidPrefixFromFilter(req.Filters)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	bidStore := prefix.NewStore(store, searchPrefix)

	pageRes, err := sdkquery.FilteredPaginate(bidStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		var bid types.Bid

		err := k.cdc.Unmarshal(value, &bid)
		if err != nil {
			return false, err
		}

		// filter bids with provided filters
		if req.Filters.Accept(bid, stateVal) {
			if accumulate {
				acct, err := k.ekeeper.GetAccount(ctx, types.EscrowAccountForBid(bid.BidID))
				if err != nil {
					return true, err
				}

				bids = append(bids, types.QueryBidResponse{
					Bid:           bid,
					EscrowAccount: acct,
				})
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

	acct, err := k.ekeeper.GetAccount(ctx, types.EscrowAccountForBid(bid.ID()))
	if err != nil {
		return nil, err
	}

	return &types.QueryBidResponse{
		Bid:           bid,
		EscrowAccount: acct,
	}, nil
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

	var leases []types.QueryLeaseResponse
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(k.skey)
	searchPrefix, isSecondaryPrefix, err := keys.LeasePrefixFromFilter(req.Filters)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	searchedStore := prefix.NewStore(store, searchPrefix)

	pageRes, err := sdkquery.FilteredPaginate(searchedStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		var lease types.Lease

		if isSecondaryPrefix {
			secondaryKey := value
			// Load the actual key, from the secondary key
			value = store.Get(secondaryKey)
		}

		err := k.cdc.Unmarshal(value, &lease)
		if err != nil {
			return false, err
		}

		// filter leases with provided filters
		if req.Filters.Accept(lease, stateVal) {
			if accumulate {
				payment, err := k.ekeeper.GetPayment(ctx,
					dtypes.EscrowAccountForDeployment(lease.ID().DeploymentID()),
					types.EscrowPaymentForLease(lease.ID()))
				if err != nil {
					return true, err
				}

				leases = append(leases, types.QueryLeaseResponse{
					Lease:         lease,
					EscrowPayment: payment,
				})
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

	payment, err := k.ekeeper.GetPayment(ctx,
		dtypes.EscrowAccountForDeployment(lease.ID().DeploymentID()),
		types.EscrowPaymentForLease(lease.ID()))
	if err != nil {
		return nil, err
	}

	return &types.QueryLeaseResponse{
		Lease:         lease,
		EscrowPayment: payment,
	}, nil
}
