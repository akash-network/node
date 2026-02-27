package keeper

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/indexes"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	"pkg.akt.dev/go/node/market/v1"
	types "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/node/util/query"
	"pkg.akt.dev/node/x/market/keeper/keys"
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
	} else if req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	ctx := sdk.UnwrapSDKContext(c)

	states := make([]byte, 0, 3)
	var resumePK *keys.OrderPrimaryKey

	// nolint: gocritic
	if len(req.Pagination.Key) > 0 {
		var pkBytes []byte
		var err error
		states, _, pkBytes, _, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		_, pk, err := k.orders.KeyCodec().Decode(pkBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resumePK = &pk
	} else if req.Filters.State != "" {
		stateVal := types.Order_State(types.Order_State_value[req.Filters.State])

		if stateVal == types.OrderStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		states = append(states, byte(types.OrderOpen), byte(types.OrderActive), byte(types.OrderClosed))
	}

	if len(req.Pagination.Key) == 0 && req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	var orders types.Orders
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

	for idx := range states {
		if req.Pagination.Limit == 0 && len(nextKey) > 0 {
			break
		}

		state := types.Order_State(states[idx])

		var iter indexes.MultiIterator[int32, keys.OrderPrimaryKey]
		var err error

		if idx == 0 && resumePK != nil {
			r := collections.NewPrefixedPairRange[int32, keys.OrderPrimaryKey](int32(state)).StartInclusive(*resumePK)
			if req.Pagination.Reverse {
				r = collections.NewPrefixedPairRange[int32, keys.OrderPrimaryKey](int32(state)).EndInclusive(*resumePK).Descending()
			}
			iter, err = k.orders.Indexes.State.Iterate(ctx, r)
		} else if req.Pagination.Reverse {
			iter, err = k.orders.Indexes.State.Iterate(ctx,
				collections.NewPrefixedPairRange[int32, keys.OrderPrimaryKey](int32(state)).Descending())
		} else {
			iter, err = k.orders.Indexes.State.MatchExact(ctx, int32(state))
		}
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		count := uint64(0)

		err = indexes.ScanValues(ctx, k.orders, iter, func(order types.Order) bool {
			if !req.Filters.Accept(order, state) {
				return false
			}

			if offset > 0 {
				offset--
				return false
			}

			if req.Pagination.Limit == 0 {
				pk := keys.OrderIDToKey(order.ID)
				pkBuf := make([]byte, k.orders.KeyCodec().Size(pk))
				if _, encErr := k.orders.KeyCodec().Encode(pkBuf, pk); encErr != nil {
					scanErr = encErr
					return true
				}
				var encErr error
				nextKey, encErr = query.EncodePaginationKey(states[idx:], []byte{states[idx]}, pkBuf, nil)
				if encErr != nil {
					scanErr = encErr
				}
				return true
			}

			orders = append(orders, order)
			req.Pagination.Limit--
			count++

			return false
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if scanErr != nil {
			return nil, status.Error(codes.Internal, scanErr.Error())
		}

		total += count
	}

	return &types.QueryOrdersResponse{
		Orders: orders,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

// Bids returns bids based on filters
func (k Querier) Bids(c context.Context, req *types.QueryBidsRequest) (*types.QueryBidsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	ctx := sdk.UnwrapSDKContext(c)

	reverseSearch := false
	states := make([]byte, 0, 4)
	var resumePK *keys.BidPrimaryKey

	// nolint: gocritic
	if len(req.Pagination.Key) > 0 {
		var pkBytes []byte
		var unsolicited []byte
		var err error
		states, _, pkBytes, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if len(unsolicited) != 1 {
			return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
		}

		if unsolicited[0] == 1 {
			reverseSearch = true
		}

		_, pk, err := k.bids.KeyCodec().Decode(pkBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resumePK = &pk
	} else if req.Filters.State != "" {
		reverseSearch = (req.Filters.Owner == "") && (req.Filters.Provider != "")

		stateVal := types.Bid_State(types.Bid_State_value[req.Filters.State])

		if stateVal == types.BidStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		states = append(states, byte(types.BidOpen), byte(types.BidActive), byte(types.BidLost), byte(types.BidClosed))
	}

	if len(req.Pagination.Key) == 0 && req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	var bids []types.QueryBidResponse
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

	encodeBidNextKey := func(bid types.Bid, idx int) {
		pk := keys.BidIDToKey(bid.ID)
		pkBuf := make([]byte, k.bids.KeyCodec().Size(pk))
		if _, encErr := k.bids.KeyCodec().Encode(pkBuf, pk); encErr != nil {
			scanErr = encErr
			return
		}
		unsolicited := []byte{0}
		if reverseSearch {
			unsolicited[0] = 1
		}
		var encErr error
		nextKey, encErr = query.EncodePaginationKey(states[idx:], []byte{states[idx]}, pkBuf, unsolicited)
		if encErr != nil {
			scanErr = encErr
		}
	}

	if reverseSearch {
		stateSet := make(map[types.Bid_State]bool)
		for _, s := range states {
			stateSet[types.Bid_State(s)] = true
		}

		var iter indexes.MultiIterator[string, keys.BidPrimaryKey]
		var err error

		if resumePK != nil {
			r := collections.NewPrefixedPairRange[string, keys.BidPrimaryKey](req.Filters.Provider).StartInclusive(*resumePK)
			if req.Pagination.Reverse {
				r = collections.NewPrefixedPairRange[string, keys.BidPrimaryKey](req.Filters.Provider).EndInclusive(*resumePK).Descending()
			}
			iter, err = k.bids.Indexes.Provider.Iterate(ctx, r)
		} else if req.Pagination.Reverse {
			iter, err = k.bids.Indexes.Provider.Iterate(ctx,
				collections.NewPrefixedPairRange[string, keys.BidPrimaryKey](req.Filters.Provider).Descending())
		} else {
			iter, err = k.bids.Indexes.Provider.MatchExact(ctx, req.Filters.Provider)
		}
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

			if offset > 0 {
				offset--
				return false
			}

			if req.Pagination.Limit == 0 {
				encodeBidNextKey(bid, 0)
				return true
			}

			acct, acctErr := k.ekeeper.GetAccount(ctx, bid.ID.ToEscrowAccountID())
			if acctErr != nil {
				scanErr = acctErr
				return true
			}

			bids = append(bids, types.QueryBidResponse{
				Bid:           bid,
				EscrowAccount: acct,
			})
			req.Pagination.Limit--
			total++

			return false
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if scanErr != nil {
			return nil, status.Error(codes.Internal, scanErr.Error())
		}
	} else {
		for idx := range states {
			if req.Pagination.Limit == 0 && len(nextKey) > 0 {
				break
			}

			state := types.Bid_State(states[idx])

			var iter indexes.MultiIterator[int32, keys.BidPrimaryKey]
			var err error

			if idx == 0 && resumePK != nil {
				r := collections.NewPrefixedPairRange[int32, keys.BidPrimaryKey](int32(state)).StartInclusive(*resumePK)
				if req.Pagination.Reverse {
					r = collections.NewPrefixedPairRange[int32, keys.BidPrimaryKey](int32(state)).EndInclusive(*resumePK).Descending()
				}
				iter, err = k.bids.Indexes.State.Iterate(ctx, r)
			} else if req.Pagination.Reverse {
				iter, err = k.bids.Indexes.State.Iterate(ctx,
					collections.NewPrefixedPairRange[int32, keys.BidPrimaryKey](int32(state)).Descending())
			} else {
				iter, err = k.bids.Indexes.State.MatchExact(ctx, int32(state))
			}
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			count := uint64(0)

			err = indexes.ScanValues(ctx, k.bids, iter, func(bid types.Bid) bool {
				if !req.Filters.Accept(bid, state) {
					return false
				}

				if offset > 0 {
					offset--
					return false
				}

				if req.Pagination.Limit == 0 {
					encodeBidNextKey(bid, idx)
					return true
				}

				acct, acctErr := k.ekeeper.GetAccount(ctx, bid.ID.ToEscrowAccountID())
				if acctErr != nil {
					scanErr = fmt.Errorf("%w: fetching escrow account for BidID=%s", acctErr, bid.ID)
					return true
				}

				bids = append(bids, types.QueryBidResponse{
					Bid:           bid,
					EscrowAccount: acct,
				})
				req.Pagination.Limit--
				count++

				return false
			})
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			if scanErr != nil {
				return nil, status.Error(codes.Internal, scanErr.Error())
			}

			total += count
		}
	}

	return &types.QueryBidsResponse{
		Bids: bids,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

// Leases returns leases based on filters
func (k Querier) Leases(c context.Context, req *types.QueryLeasesRequest) (*types.QueryLeasesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	ctx := sdk.UnwrapSDKContext(c)

	reverseSearch := false
	states := make([]byte, 0, 3)
	var resumePK *keys.LeasePrimaryKey

	// nolint: gocritic
	if len(req.Pagination.Key) > 0 {
		var pkBytes []byte
		var unsolicited []byte
		var err error
		states, _, pkBytes, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if len(unsolicited) != 1 {
			return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
		}

		if unsolicited[0] == 1 {
			reverseSearch = true
		}

		_, pk, err := k.leases.KeyCodec().Decode(pkBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resumePK = &pk
	} else if req.Filters.State != "" {
		reverseSearch = (req.Filters.Owner == "") && (req.Filters.Provider != "")

		stateVal := v1.Lease_State(v1.Lease_State_value[req.Filters.State])

		if stateVal == v1.LeaseStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		states = append(states, byte(v1.LeaseActive), byte(v1.LeaseInsufficientFunds), byte(v1.LeaseClosed))
	}

	if len(req.Pagination.Key) == 0 && req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	var leases []types.QueryLeaseResponse
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

	encodeLeaseNextKey := func(lease v1.Lease, idx int) {
		pk := keys.LeaseIDToKey(lease.ID)
		pkBuf := make([]byte, k.leases.KeyCodec().Size(pk))
		if _, encErr := k.leases.KeyCodec().Encode(pkBuf, pk); encErr != nil {
			scanErr = encErr
			return
		}
		unsolicited := []byte{0}
		if reverseSearch {
			unsolicited[0] = 1
		}
		var encErr error
		nextKey, encErr = query.EncodePaginationKey(states[idx:], []byte{states[idx]}, pkBuf, unsolicited)
		if encErr != nil {
			scanErr = encErr
		}
	}

	if reverseSearch {
		stateSet := make(map[v1.Lease_State]bool)
		for _, s := range states {
			stateSet[v1.Lease_State(s)] = true
		}

		var iter indexes.MultiIterator[string, keys.LeasePrimaryKey]
		var err error

		if resumePK != nil {
			r := collections.NewPrefixedPairRange[string, keys.LeasePrimaryKey](req.Filters.Provider).StartInclusive(*resumePK)
			if req.Pagination.Reverse {
				r = collections.NewPrefixedPairRange[string, keys.LeasePrimaryKey](req.Filters.Provider).EndInclusive(*resumePK).Descending()
			}
			iter, err = k.leases.Indexes.Provider.Iterate(ctx, r)
		} else if req.Pagination.Reverse {
			iter, err = k.leases.Indexes.Provider.Iterate(ctx,
				collections.NewPrefixedPairRange[string, keys.LeasePrimaryKey](req.Filters.Provider).Descending())
		} else {
			iter, err = k.leases.Indexes.Provider.MatchExact(ctx, req.Filters.Provider)
		}
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

			if offset > 0 {
				offset--
				return false
			}

			if req.Pagination.Limit == 0 {
				encodeLeaseNextKey(lease, 0)
				return true
			}

			payment, pmntErr := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
			if pmntErr != nil {
				scanErr = pmntErr
				return true
			}

			leases = append(leases, types.QueryLeaseResponse{
				Lease:         lease,
				EscrowPayment: payment,
			})
			req.Pagination.Limit--
			total++

			return false
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if scanErr != nil {
			return nil, status.Error(codes.Internal, scanErr.Error())
		}
	} else {
		for idx := range states {
			if req.Pagination.Limit == 0 && len(nextKey) > 0 {
				break
			}

			state := v1.Lease_State(states[idx])

			var iter indexes.MultiIterator[int32, keys.LeasePrimaryKey]
			var err error

			if idx == 0 && resumePK != nil {
				r := collections.NewPrefixedPairRange[int32, keys.LeasePrimaryKey](int32(state)).StartInclusive(*resumePK)
				if req.Pagination.Reverse {
					r = collections.NewPrefixedPairRange[int32, keys.LeasePrimaryKey](int32(state)).EndInclusive(*resumePK).Descending()
				}
				iter, err = k.leases.Indexes.State.Iterate(ctx, r)
			} else if req.Pagination.Reverse {
				iter, err = k.leases.Indexes.State.Iterate(ctx,
					collections.NewPrefixedPairRange[int32, keys.LeasePrimaryKey](int32(state)).Descending())
			} else {
				iter, err = k.leases.Indexes.State.MatchExact(ctx, int32(state))
			}
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			count := uint64(0)

			err = indexes.ScanValues(ctx, k.leases, iter, func(lease v1.Lease) bool {
				if !req.Filters.Accept(lease, state) {
					return false
				}

				if offset > 0 {
					offset--
					return false
				}

				if req.Pagination.Limit == 0 {
					encodeLeaseNextKey(lease, idx)
					return true
				}

				payment, pmntErr := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
				if pmntErr != nil {
					scanErr = pmntErr
					return true
				}

				leases = append(leases, types.QueryLeaseResponse{
					Lease:         lease,
					EscrowPayment: payment,
				})
				req.Pagination.Limit--
				count++

				return false
			})
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			if scanErr != nil {
				return nil, status.Error(codes.Internal, scanErr.Error())
			}

			total += count
		}
	}

	return &types.QueryLeasesResponse{
		Leases: leases,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
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
