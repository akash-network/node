package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mv1 "pkg.akt.dev/go/node/market/v1"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	mtypes "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/node/v2/util/query"
	"pkg.akt.dev/node/v2/x/market/keeper/keys"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	Keeper
}

var _ mtypes.QueryServer = Querier{}

// Orders returns orders based on filters
func (k Querier) Orders(c context.Context, req *mtypes.QueryOrdersRequest) (*mtypes.QueryOrdersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	// case 1: no filters set, iterating over entire store
	// case 2: state only or state plus underlying filters like owner, iterating over state store
	// case 3: state not set, underlying filters like owner are set, most complex case

	states := make([]byte, 0, 3)
	var searchPrefix []byte

	// setup for case 3 - cross-index search
	// nolint: gocritic
	if len(req.Pagination.Key) > 0 {
		var key []byte
		var err error
		states, searchPrefix, key, _, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		req.Pagination.Key = key
	} else if req.Filters.State != "" {
		stateVal := mtypes.Order_State(mtypes.Order_State_value[req.Filters.State])

		if req.Filters.State != "" && stateVal == mtypes.OrderStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		// request does not have a pagination set. Start from an open store
		states = append(states, []byte{byte(mtypes.OrderOpen), byte(mtypes.OrderActive), byte(mtypes.OrderClosed)}...)
	}

	var orders mtypes.Orders
	var pageRes *sdkquery.PageResponse

	ctx := sdk.UnwrapSDKContext(c)

	total := uint64(0)

	var idx int
	var err error

	for idx = range states {
		state := mtypes.Order_State(states[idx])
		if idx > 0 {
			req.Pagination.Key = nil
		}

		if len(req.Pagination.Key) == 0 {
			req.Filters.State = state.String()

			searchPrefix, err = keys.OrderPrefixFromFilter(req.Filters)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

		searchStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

		count := uint64(0)

		pageRes, err = sdkquery.FilteredPaginate(searchStore, req.Pagination, func(_ []byte, value []byte, accumulate bool) (bool, error) {
			var order mtypes.Order

			err := k.cdc.Unmarshal(value, &order)
			if err != nil {
				return false, err
			}

			// filter orders with provided filters
			if req.Filters.Accept(order, state) {
				if accumulate {
					orders = append(orders, order)
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

	if pageRes != nil {
		pageRes.Total = total

		if len(pageRes.NextKey) > 0 {
			pageRes.NextKey, err = query.EncodePaginationKey(states[idx:], searchPrefix, pageRes.NextKey, nil)
			if err != nil {
				pageRes.Total = total
				return &mtypes.QueryOrdersResponse{
					Orders:     orders,
					Pagination: pageRes,
				}, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return &mtypes.QueryOrdersResponse{
		Orders:     orders,
		Pagination: pageRes,
	}, nil
}

// Bids returns bids based on filters
func (k Querier) Bids(c context.Context, req *mtypes.QueryBidsRequest) (*mtypes.QueryBidsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	reverseSearch := false
	states := make([]byte, 0, 4)
	var searchPrefix []byte

	// setup for case 3 - cross-index search
	// nolint: gocritic
	if len(req.Pagination.Key) > 0 {
		var key []byte
		var unsolicited []byte
		var err error
		states, searchPrefix, key, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(unsolicited) != 1 {
			return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
		}
		req.Pagination.Key = key

		if unsolicited[0] == 1 {
			reverseSearch = true
		}
	} else if req.Filters.State != "" {
		reverseSearch = (req.Filters.Owner == "") && (req.Filters.Provider != "")

		stateVal := mtypes.Bid_State(mtypes.Bid_State_value[req.Filters.State])

		if req.Filters.State != "" && stateVal == mtypes.BidStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		// request does not have a pagination set. Start from an open store
		states = append(states, byte(mtypes.BidOpen), byte(mtypes.BidActive), byte(mtypes.BidLost), byte(mtypes.BidClosed))
	}

	var bids []mtypes.QueryBidResponse
	var pageRes *sdkquery.PageResponse
	ctx := sdk.UnwrapSDKContext(c)

	total := uint64(0)

	var idx int
	var err error

	for idx = range states {
		state := mtypes.Bid_State(states[idx])

		if idx > 0 {
			req.Pagination.Key = nil
		}

		if len(req.Pagination.Key) == 0 {
			req.Filters.State = state.String()

			if reverseSearch {
				searchPrefix, err = keys.BidReversePrefixFromFilter(req.Filters)
			} else {
				searchPrefix, err = keys.BidPrefixFromFilter(req.Filters)
			}

			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

		count := uint64(0)
		searchStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

		pageRes, err = sdkquery.FilteredPaginate(searchStore, req.Pagination, func(_ []byte, value []byte, accumulate bool) (bool, error) {
			var bid mtypes.Bid

			err := k.cdc.Unmarshal(value, &bid)
			if err != nil {
				return false, err
			}

			// filter bids with provided filters
			if req.Filters.Accept(bid, state) {
				if accumulate {
					acct, err := k.ekeeper.GetAccount(ctx, bid.ID.ToEscrowAccountID())
					if err != nil {
						return true, err
					}

					bids = append(bids, mtypes.QueryBidResponse{
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

	if pageRes != nil {
		pageRes.Total = total

		if len(pageRes.NextKey) > 0 {
			unsolicited := make([]byte, 1)
			unsolicited[0] = 0
			if reverseSearch {
				unsolicited[0] = 1
			}

			pageRes.NextKey, err = query.EncodePaginationKey(states[idx:], searchPrefix, pageRes.NextKey, unsolicited)
			if err != nil {
				pageRes.Total = total
				return &mtypes.QueryBidsResponse{
					Bids:       bids,
					Pagination: pageRes,
				}, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return &mtypes.QueryBidsResponse{
		Bids:       bids,
		Pagination: pageRes,
	}, nil
}

// Leases returns leases based on filters
func (k Querier) Leases(c context.Context, req *mtypes.QueryLeasesRequest) (*mtypes.QueryLeasesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	// setup for case 3 - cross-index search
	reverseSearch := false
	states := make([]byte, 0, 3)
	var searchPrefix []byte

	// setup for case 3 - cross-index search
	// nolint: gocritic
	if len(req.Pagination.Key) > 0 {
		var key []byte
		var unsolicited []byte
		var err error
		states, searchPrefix, key, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(unsolicited) != 1 {
			return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
		}
		req.Pagination.Key = key

		if unsolicited[0] == 1 {
			reverseSearch = true
		}
	} else if req.Filters.State != "" {
		reverseSearch = (req.Filters.Owner == "") && (req.Filters.Provider != "")

		stateVal := mv1.Lease_State(mv1.Lease_State_value[req.Filters.State])

		if req.Filters.State != "" && stateVal == mv1.LeaseStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		// request does not have a pagination set. Start from an open store
		states = append(states, byte(mv1.LeaseActive), byte(mv1.LeaseInsufficientFunds), byte(mv1.LeaseClosed))
	}

	var leases []mtypes.QueryLeaseResponse
	var pageRes *sdkquery.PageResponse
	ctx := sdk.UnwrapSDKContext(c)

	total := uint64(0)

	var idx int
	var err error

	for idx = range states {
		state := mv1.Lease_State(states[idx])

		if idx > 0 {
			req.Pagination.Key = nil
		}

		if len(req.Pagination.Key) == 0 {
			req.Filters.State = state.String()

			if reverseSearch {
				searchPrefix, err = keys.LeaseReversePrefixFromFilter(req.Filters)
			} else {
				searchPrefix, err = keys.LeasePrefixFromFilter(req.Filters)
			}

			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

		searchedStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

		count := uint64(0)

		pageRes, err = sdkquery.FilteredPaginate(searchedStore, req.Pagination, func(_ []byte, value []byte, accumulate bool) (bool, error) {
			var lease mv1.Lease

			err := k.cdc.Unmarshal(value, &lease)
			if err != nil {
				return false, err
			}

			// filter leases with provided filters
			if req.Filters.Accept(lease, state) {
				if accumulate {
					payment, err := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
					if err != nil {
						return true, err
					}

					leases = append(leases, mtypes.QueryLeaseResponse{
						Lease:         lease,
						EscrowPayment: payment,
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

	if pageRes != nil {
		pageRes.Total = total

		if len(pageRes.NextKey) > 0 {
			unsolicited := make([]byte, 1)
			unsolicited[0] = 0
			if reverseSearch {
				unsolicited[0] = 1
			}

			pageRes.NextKey, err = query.EncodePaginationKey(states[idx:], searchPrefix, pageRes.NextKey, unsolicited)
			if err != nil {
				pageRes.Total = total
				return &mtypes.QueryLeasesResponse{
					Leases:     leases,
					Pagination: pageRes,
				}, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return &mtypes.QueryLeasesResponse{
		Leases:     leases,
		Pagination: pageRes,
	}, nil
}

// Order returns order details based on OrderID
func (k Querier) Order(c context.Context, req *mtypes.QueryOrderRequest) (*mtypes.QueryOrderResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if _, err := sdk.AccAddressFromBech32(req.ID.Owner); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner address")
	}

	ctx := sdk.UnwrapSDKContext(c)

	order, found := k.GetOrder(ctx, req.ID)
	if !found {
		return nil, mv1.ErrOrderNotFound
	}

	return &mtypes.QueryOrderResponse{Order: order}, nil
}

// Bid returns bid details based on BidID
func (k Querier) Bid(c context.Context, req *mtypes.QueryBidRequest) (*mtypes.QueryBidResponse, error) {
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
		return nil, mv1.ErrBidNotFound
	}

	acct, err := k.ekeeper.GetAccount(ctx, bid.ID.ToEscrowAccountID())
	if err != nil {
		return nil, err
	}

	return &mtypes.QueryBidResponse{
		Bid:           bid,
		EscrowAccount: acct,
	}, nil
}

// Lease returns lease details based on LeaseID
func (k Querier) Lease(c context.Context, req *mtypes.QueryLeaseRequest) (*mtypes.QueryLeaseResponse, error) {
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
		return nil, mv1.ErrLeaseNotFound
	}

	payment, err := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
	if err != nil {
		return nil, err
	}

	return &mtypes.QueryLeaseResponse{
		Lease:         lease,
		EscrowPayment: payment,
	}, nil
}

func (k Querier) Params(ctx context.Context, req *mtypes.QueryParamsRequest) (*mtypes.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params := k.GetParams(sdkCtx)

	return &mtypes.QueryParamsResponse{Params: params}, nil
}
