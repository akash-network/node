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

	// case 1: no filters set, iterating over entire store
	// case 2: state only or state plus underlying filters like owner, iterating over state store
	// case 3: state not set, underlying filters like owner are set, most complex case

	var store prefix.Store

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	states := make([]types.Order_State, 0, 3)

	// setup for case 3 - cross-index search
	if req.Filters.State == "" {
		// request has pagination key set, determine store prefix
		if len(req.Pagination.Key) > 0 {
			if len(req.Pagination.Key) < 3 {
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}

			switch req.Pagination.Key[2] {
			case keys.OrderStateOpenPrefixID:
				states = append(states, types.OrderOpen)
				fallthrough
			case keys.OrderStateActivePrefixID:
				states = append(states, types.OrderActive)
				fallthrough
			case keys.OrderStateClosedPrefixID:
				states = append(states, types.OrderClosed)
			default:
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}
		} else {
			// request does not have pagination set. Start from open store
			states = append(states, types.OrderOpen)
			states = append(states, types.OrderActive)
			states = append(states, types.OrderClosed)
		}
	} else {
		states = append(states, stateVal)
	}

	var orders types.Orders
	var pageRes *sdkquery.PageResponse

	ctx := sdk.UnwrapSDKContext(c)

	total := uint64(0)

	for _, state := range states {
		var searchPrefix []byte
		var err error

		req.Filters.State = state.String()

		searchPrefix, err = keys.OrderPrefixFromFilter(req.Filters)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		store = prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

		count := uint64(0)

		pageRes, err = sdkquery.FilteredPaginate(store, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
			var order types.Order

			err := k.cdc.Unmarshal(value, &order)
			if err != nil {
				return false, err
			}

			// filter orders with provided filters
			if req.Filters.Accept(order, stateVal) && accumulate {
				orders = append(orders, order)
				count++

				return true, nil
			}

			return false, nil
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		req.Pagination.Limit -= count
		total += count

		if req.Pagination.Limit == 0 {
			break
		}
	}

	pageRes.Total = total

	return &types.QueryOrdersResponse{
		Orders:     orders,
		Pagination: pageRes,
	}, nil
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

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	reverseSearch := (req.Filters.Owner == "") && (req.Filters.Provider != "")
	states := make([]types.Bid_State, 0, 4)

	// setup for case 3 - cross-index search
	if req.Filters.State == "" {
		// request has pagination key set, determine store prefix
		if len(req.Pagination.Key) > 0 {
			if len(req.Pagination.Key) < 3 {
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}

			if reverseSearch && req.Pagination.Key[2] > keys.BidStateActivePrefixID {
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}

			switch req.Pagination.Key[2] {
			case keys.BidStateOpenPrefixID:
				states = append(states, types.BidOpen)
				fallthrough
			case keys.BidStateActivePrefixID:
				states = append(states, types.BidActive)
				fallthrough
			case keys.BidStateLostPrefixID:
				states = append(states, types.BidLost)
				fallthrough
			case keys.BidStateClosedPrefixID:
				states = append(states, types.BidClosed)
			default:
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}
		} else {
			// request does not have pagination set. Start from open store
			states = append(states, types.BidOpen, types.BidActive, types.BidLost, types.BidClosed)
		}
	} else {
		states = append(states, stateVal)
	}

	var bids []types.QueryBidResponse
	var pageRes *sdkquery.PageResponse
	ctx := sdk.UnwrapSDKContext(c)

	total := uint64(0)

	for _, state := range states {
		var searchPrefix []byte
		var err error

		req.Filters.State = state.String()

		if reverseSearch {
			searchPrefix, err = keys.BidReversePrefixFromFilter(req.Filters)
		} else {
			searchPrefix, err = keys.BidPrefixFromFilter(req.Filters)
		}

		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		count := uint64(0)

		bidStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)
		pageRes, err = sdkquery.FilteredPaginate(bidStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
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

					count++
				}

				return true, nil
			}

			return false, nil
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		req.Pagination.Limit -= count
		total += count

		if req.Pagination.Limit == 0 {
			break
		}
	}

	pageRes.Total = total

	return &types.QueryBidsResponse{
		Bids:       bids,
		Pagination: pageRes,
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

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	reverseSearch := (req.Filters.Owner == "") && (req.Filters.Provider != "")

	states := make([]types.Lease_State, 0, 3)

	// setup for case 3 - cross-index search
	if req.Filters.State == "" {
		// request has pagination key set, determine store prefix
		if len(req.Pagination.Key) > 0 {
			if len(req.Pagination.Key) < 3 {
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}

			if reverseSearch && req.Pagination.Key[2] > keys.LeaseStateActivePrefixID {
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}

			switch req.Pagination.Key[2] {
			case keys.LeaseStateActivePrefixID:
				states = append(states, types.LeaseActive)
				fallthrough
			case keys.LeaseStateInsufficientFundsPrefixID:
				states = append(states, types.LeaseInsufficientFunds)
				fallthrough
			case keys.LeaseStateClosedPrefixID:
				states = append(states, types.LeaseClosed)
			default:
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}
		} else {
			// request does not have pagination set. Start from open store
			req.Filters.State = types.LeaseActive.String()
			states = append(states, types.LeaseActive, types.LeaseInsufficientFunds, types.LeaseClosed)
		}
	} else {
		states = append(states, stateVal)
	}

	var leases []types.QueryLeaseResponse
	var pageRes *sdkquery.PageResponse
	ctx := sdk.UnwrapSDKContext(c)

	total := uint64(0)

	for _, state := range states {
		var searchPrefix []byte
		var err error

		req.Filters.State = state.String()

		if reverseSearch {
			searchPrefix, err = keys.LeaseReversePrefixFromFilter(req.Filters)
		} else {
			searchPrefix, err = keys.LeasePrefixFromFilter(req.Filters)
		}

		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		searchedStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

		count := uint64(0)

		pageRes, err = sdkquery.FilteredPaginate(searchedStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
			var lease types.Lease

			err := k.cdc.Unmarshal(value, &lease)
			if err != nil {
				return false, err
			}

			// filter leases with provided filters
			if req.Filters.Accept(lease, stateVal) && accumulate {
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

				count++
				return true, nil
			}

			return false, nil
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		req.Pagination.Limit -= count
		total += count

		if req.Pagination.Limit == 0 {
			break
		}
	}

	pageRes.Total = total

	return &types.QueryLeasesResponse{
		Leases:     leases,
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
