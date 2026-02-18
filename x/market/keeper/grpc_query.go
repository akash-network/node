package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections/indexes"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	"pkg.akt.dev/go/node/market/v1"
	types "pkg.akt.dev/go/node/market/v1beta5"
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

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	if len(req.Pagination.Key) > 0 || req.Pagination.Reverse {
		return nil, status.Error(codes.InvalidArgument, "key-based and reverse pagination are not supported")
	}

	ctx := sdk.UnwrapSDKContext(c)

	// Determine which states to iterate
	states := []types.Order_State{types.OrderOpen, types.OrderActive, types.OrderClosed}
	if req.Filters.State != "" {
		stateVal := types.Order_State(types.Order_State_value[req.Filters.State])
		if stateVal == types.OrderStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}
		states = []types.Order_State{stateVal}
	}

	var orders types.Orders
	limit := req.Pagination.Limit
	offset := req.Pagination.Offset
	skipped := uint64(0)
	countTotal := req.Pagination.CountTotal
	var total uint64

	for _, state := range states {
		if limit == 0 && !countTotal {
			break
		}

		iter, err := k.orders.Indexes.State.MatchExact(ctx, int32(state))
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		err = indexes.ScanValues(ctx, k.orders, iter, func(order types.Order) bool {
			if !req.Filters.Accept(order, state) {
				return false
			}

			if countTotal {
				total++
			}

			if limit == 0 {
				return !countTotal
			}

			if skipped < offset {
				skipped++
				return false
			}

			orders = append(orders, order)
			limit--

			return false
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	resp := &types.QueryOrdersResponse{
		Orders:     orders,
		Pagination: &sdkquery.PageResponse{},
	}

	if countTotal {
		resp.Pagination.Total = total
	}

	return resp, nil
}

// Bids returns bids based on filters
func (k Querier) Bids(c context.Context, req *types.QueryBidsRequest) (*types.QueryBidsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	if len(req.Pagination.Key) > 0 || req.Pagination.Reverse {
		return nil, status.Error(codes.InvalidArgument, "key-based and reverse pagination are not supported")
	}

	ctx := sdk.UnwrapSDKContext(c)

	// Determine which states to iterate
	states := []types.Bid_State{types.BidOpen, types.BidActive, types.BidLost, types.BidClosed}
	if req.Filters.State != "" {
		stateVal := types.Bid_State(types.Bid_State_value[req.Filters.State])
		if stateVal == types.BidStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}
		states = []types.Bid_State{stateVal}
	}

	var bids []types.QueryBidResponse
	limit := req.Pagination.Limit
	offset := req.Pagination.Offset
	skipped := uint64(0)
	countTotal := req.Pagination.CountTotal
	var total uint64

	// Use Provider index when filtering by provider without owner
	providerSearch := req.Filters.Owner == "" && req.Filters.Provider != ""
	var acctErr error

	if providerSearch {
		stateSet := make(map[types.Bid_State]bool)
		for _, s := range states {
			stateSet[s] = true
		}

		iter, err := k.bids.Indexes.Provider.MatchExact(ctx, req.Filters.Provider)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		err = indexes.ScanValues(ctx, k.bids, iter, func(bid types.Bid) bool {
			if !stateSet[bid.State] {
				return false
			}

			if !req.Filters.Accept(bid, bid.State) {
				return false
			}

			if countTotal {
				total++
			}

			if limit == 0 {
				return !countTotal
			}

			if skipped < offset {
				skipped++
				return false
			}

			acct, acctE := k.ekeeper.GetAccount(ctx, bid.ID.ToEscrowAccountID())
			if acctE != nil {
				acctErr = acctE
				return true
			}

			bids = append(bids, types.QueryBidResponse{
				Bid:           bid,
				EscrowAccount: acct,
			})
			limit--

			return false
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if acctErr != nil {
			return nil, status.Error(codes.Internal, acctErr.Error())
		}
	} else {
		for _, state := range states {
			if limit == 0 && !countTotal {
				break
			}

			iter, err := k.bids.Indexes.State.MatchExact(ctx, int32(state))
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			err = indexes.ScanValues(ctx, k.bids, iter, func(bid types.Bid) bool {
				if !req.Filters.Accept(bid, state) {
					return false
				}

				if countTotal {
					total++
				}

				if limit == 0 {
					return !countTotal
				}

				if skipped < offset {
					skipped++
					return false
				}

				acct, acctE := k.ekeeper.GetAccount(ctx, bid.ID.ToEscrowAccountID())
				if acctE != nil {
					acctErr = acctE
					return true
				}

				bids = append(bids, types.QueryBidResponse{
					Bid:           bid,
					EscrowAccount: acct,
				})
				limit--

				return false
			})
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			if acctErr != nil {
				return nil, status.Error(codes.Internal, acctErr.Error())
			}
		}
	}

	resp := &types.QueryBidsResponse{
		Bids:       bids,
		Pagination: &sdkquery.PageResponse{},
	}

	if countTotal {
		resp.Pagination.Total = total
	}

	return resp, nil
}

// Leases returns leases based on filters
func (k Querier) Leases(c context.Context, req *types.QueryLeasesRequest) (*types.QueryLeasesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	if len(req.Pagination.Key) > 0 || req.Pagination.Reverse {
		return nil, status.Error(codes.InvalidArgument, "key-based and reverse pagination are not supported")
	}

	ctx := sdk.UnwrapSDKContext(c)

	// Determine which states to iterate
	states := []v1.Lease_State{v1.LeaseActive, v1.LeaseInsufficientFunds, v1.LeaseClosed}
	if req.Filters.State != "" {
		stateVal := v1.Lease_State(v1.Lease_State_value[req.Filters.State])
		if stateVal == v1.LeaseStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}
		states = []v1.Lease_State{stateVal}
	}

	var leases []types.QueryLeaseResponse
	limit := req.Pagination.Limit
	offset := req.Pagination.Offset
	skipped := uint64(0)
	countTotal := req.Pagination.CountTotal
	var total uint64

	// Use Provider index when filtering by provider without owner
	providerSearch := req.Filters.Owner == "" && req.Filters.Provider != ""
	var pmntErr error

	if providerSearch {
		stateSet := make(map[v1.Lease_State]bool)
		for _, s := range states {
			stateSet[s] = true
		}

		iter, err := k.leases.Indexes.Provider.MatchExact(ctx, req.Filters.Provider)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		err = indexes.ScanValues(ctx, k.leases, iter, func(lease v1.Lease) bool {
			if !stateSet[lease.State] {
				return false
			}

			if !req.Filters.Accept(lease, lease.State) {
				return false
			}

			if countTotal {
				total++
			}

			if limit == 0 {
				return !countTotal
			}

			if skipped < offset {
				skipped++
				return false
			}

			payment, pmntE := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
			if pmntE != nil {
				pmntErr = pmntE
				return true
			}

			leases = append(leases, types.QueryLeaseResponse{
				Lease:         lease,
				EscrowPayment: payment,
			})
			limit--

			return false
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if pmntErr != nil {
			return nil, status.Error(codes.Internal, pmntErr.Error())
		}
	} else {
		for _, state := range states {
			if limit == 0 && !countTotal {
				break
			}

			iter, err := k.leases.Indexes.State.MatchExact(ctx, int32(state))
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			err = indexes.ScanValues(ctx, k.leases, iter, func(lease v1.Lease) bool {
				if !req.Filters.Accept(lease, state) {
					return false
				}

				if countTotal {
					total++
				}

				if limit == 0 {
					return !countTotal
				}

				if skipped < offset {
					skipped++
					return false
				}

				payment, pmntE := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
				if pmntE != nil {
					pmntErr = pmntE
					return true
				}

				leases = append(leases, types.QueryLeaseResponse{
					Lease:         lease,
					EscrowPayment: payment,
				})
				limit--

				return false
			})
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			if pmntErr != nil {
				return nil, status.Error(codes.Internal, pmntErr.Error())
			}
		}
	}

	resp := &types.QueryLeasesResponse{
		Leases:     leases,
		Pagination: &sdkquery.PageResponse{},
	}

	if countTotal {
		resp.Pagination.Total = total
	}

	return resp, nil
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
		return nil, v1.ErrOrderNotFound
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
		return nil, v1.ErrBidNotFound
	}

	acct, err := k.ekeeper.GetAccount(ctx, bid.ID.ToEscrowAccountID())
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
		return nil, v1.ErrLeaseNotFound
	}

	payment, err := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
	if err != nil {
		return nil, err
	}

	return &types.QueryLeaseResponse{
		Lease:         lease,
		EscrowPayment: payment,
	}, nil
}

func (k Querier) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params := k.GetParams(sdkCtx)

	return &types.QueryParamsResponse{Params: params}, nil
}
