package keeper

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/indexes"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	v1 "pkg.akt.dev/go/node/market/v1"
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

	// Step 1: Resolve states, resumePK, and iteration path
	states := make([]byte, 0, 3)
	var resumePK *keys.OrderPrimaryKey
	var ownerPath bool
	var owner string

	if len(req.Pagination.Key) > 0 {
		// RESUME — all filters ignored, key provides everything
		var pkBytes, unsolicited []byte
		var err error
		states, _, pkBytes, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		_, pk, err := k.orders.KeyCodec().Decode(pkBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resumePK = &pk

		if len(unsolicited) > 0 && unsolicited[0] == 1 {
			ownerPath = true
			owner = resumePK.K1()
		}
	} else {
		// INITIAL — resolve states from Filters.State
		if req.Filters.State != "" {
			stateVal := types.Order_State(types.Order_State_value[req.Filters.State])
			if stateVal == types.OrderStateInvalid {
				return nil, status.Error(codes.InvalidArgument, "invalid state value")
			}
			states = append(states, byte(stateVal))
		} else {
			states = append(states, byte(types.OrderOpen), byte(types.OrderActive), byte(types.OrderClosed))
		}

		// Resolve iteration path from Filters.Owner
		if req.Filters.Owner != "" {
			ownerPath = true
			owner = req.Filters.Owner
		}
	}

	// Step 2: Direct Get — all 4 ID fields set
	if ownerPath && resumePK == nil && req.Filters.DSeq != 0 && req.Filters.GSeq != 0 && req.Filters.OSeq != 0 {
		return k.ordersDirectGet(ctx, req, states)
	}

	// Step 3: Owner path — iterate primary map with owner prefix
	if ownerPath {
		return k.ordersOwnerPath(ctx, req, states, owner, resumePK)
	}

	// Step 4: State-index path
	if len(req.Pagination.Key) == 0 && req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	return k.ordersStatePath(ctx, req, states, resumePK)
}

// ordersDirectGet handles the case where all 4 ID fields are set, giving a full primary key.
func (k Querier) ordersDirectGet(
	ctx sdk.Context,
	req *types.QueryOrdersRequest,
	states []byte,
) (*types.QueryOrdersResponse, error) {
	pk := collections.Join4(req.Filters.Owner, req.Filters.DSeq, req.Filters.GSeq, req.Filters.OSeq)
	order, err := k.orders.Get(ctx, pk)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return &types.QueryOrdersResponse{
				Pagination: &sdkquery.PageResponse{},
			}, nil
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check state filter
	stateMatch := false
	for _, s := range states {
		if types.Order_State(s) == order.State {
			stateMatch = true
			break
		}
	}
	if !stateMatch {
		return &types.QueryOrdersResponse{
			Pagination: &sdkquery.PageResponse{},
		}, nil
	}

	return &types.QueryOrdersResponse{
		Orders: types.Orders{order},
		Pagination: &sdkquery.PageResponse{
			Total: 1,
		},
	}, nil
}

// ordersOwnerPath iterates the primary map with an owner prefix.
func (k Querier) ordersOwnerPath(
	ctx sdk.Context,
	req *types.QueryOrdersRequest,
	states []byte,
	owner string,
	resumePK *keys.OrderPrimaryKey,
) (*types.QueryOrdersResponse, error) {
	// Build state set for callback filtering
	stateSet := make(map[types.Order_State]bool, len(states))
	for _, s := range states {
		stateSet[types.Order_State(s)] = true
	}

	// Build range on primary map
	prefix := collections.QuadPrefix[string, uint64, uint32, uint32](owner)
	r := new(collections.Range[keys.OrderPrimaryKey]).Prefix(prefix)
	if resumePK != nil {
		if req.Pagination.Reverse {
			r.EndInclusive(*resumePK).Descending()
		} else {
			r.StartInclusive(*resumePK)
		}
	} else if req.Pagination.Reverse {
		r.Descending()
	}

	var orders types.Orders
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset

	walkErr := k.orders.Walk(ctx, r, func(_ keys.OrderPrimaryKey, order types.Order) (bool, error) {
		if !stateSet[order.State] {
			return false, nil
		}

		if !req.Filters.Accept(order, order.State) {
			return false, nil
		}

		if offset > 0 {
			offset--
			return false, nil
		}

		if req.Pagination.Limit == 0 {
			npk := keys.OrderIDToKey(order.ID)
			pkBuf := make([]byte, k.orders.KeyCodec().Size(npk))
			if _, err := k.orders.KeyCodec().Encode(pkBuf, npk); err != nil {
				return true, err
			}
			var err error
			nextKey, err = query.EncodePaginationKey(states, []byte{states[0]}, pkBuf, []byte{1})
			if err != nil {
				return true, err
			}
			return true, nil
		}

		orders = append(orders, order)
		req.Pagination.Limit--
		total++
		return false, nil
	})
	if walkErr != nil {
		return nil, status.Error(codes.Internal, walkErr.Error())
	}

	return &types.QueryOrdersResponse{
		Orders: orders,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

// ordersStatePath iterates orders via the State index.
func (k Querier) ordersStatePath(
	ctx sdk.Context,
	req *types.QueryOrdersRequest,
	states []byte,
	resumePK *keys.OrderPrimaryKey,
) (*types.QueryOrdersResponse, error) {
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

	// Step 1: Resolve states, resumePK, and iteration path
	// pathType: 0=state-index, 1=provider-index, 2=owner-prefix
	states := make([]byte, 0, 4)
	var resumePK *keys.BidPrimaryKey
	pathType := byte(0)
	var owner string
	var provider string

	if len(req.Pagination.Key) > 0 {
		// RESUME — all filters ignored, key provides everything
		var pkBytes, unsolicited []byte
		var err error
		states, _, pkBytes, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if len(unsolicited) != 1 {
			return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
		}

		pathType = unsolicited[0]

		_, pk, err := k.bids.KeyCodec().Decode(pkBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resumePK = &pk

		switch pathType {
		case 1: // provider path — recover provider from PK
			provider = resumePK.K2().K1()
		case 2: // owner path — recover owner from PK
			owner = resumePK.K1().K1()
		}
	} else {
		// INITIAL — resolve states from Filters.State
		if req.Filters.State != "" {
			stateVal := types.Bid_State(types.Bid_State_value[req.Filters.State])
			if stateVal == types.BidStateInvalid {
				return nil, status.Error(codes.InvalidArgument, "invalid state value")
			}
			states = append(states, byte(stateVal))
		} else {
			states = append(states, byte(types.BidOpen), byte(types.BidActive), byte(types.BidLost), byte(types.BidClosed))
		}

		// Resolve iteration path from filters
		if req.Filters.Owner != "" {
			pathType = 2
			owner = req.Filters.Owner
		} else if req.Filters.Provider != "" {
			pathType = 1
			provider = req.Filters.Provider
		}
	}

	// Step 2: Owner path — iterate primary map with owner prefix
	if pathType == 2 {
		return k.bidsOwnerPath(ctx, req, states, owner, resumePK)
	}

	// Step 3: Provider path — iterate provider index
	if pathType == 1 {
		return k.bidsProviderPath(ctx, req, states, provider, resumePK)
	}

	// Step 4: State-index path
	if len(req.Pagination.Key) == 0 && req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	return k.bidsStatePath(ctx, req, states, resumePK)
}

// bidsOwnerPath iterates the primary map with an owner prefix.
func (k Querier) bidsOwnerPath(
	ctx sdk.Context,
	req *types.QueryBidsRequest,
	states []byte,
	owner string,
	resumePK *keys.BidPrimaryKey,
) (*types.QueryBidsResponse, error) {
	stateSet := make(map[types.Bid_State]bool, len(states))
	for _, s := range states {
		stateSet[types.Bid_State(s)] = true
	}

	orderPrefix := collections.QuadPrefix[string, uint64, uint32, uint32](owner)
	bidPrefix := collections.PairPrefix[keys.OrderPrimaryKey, keys.ProviderPartKey](orderPrefix)
	r := new(collections.Range[keys.BidPrimaryKey]).Prefix(bidPrefix)
	if resumePK != nil {
		if req.Pagination.Reverse {
			r.EndInclusive(*resumePK).Descending()
		} else {
			r.StartInclusive(*resumePK)
		}
	} else if req.Pagination.Reverse {
		r.Descending()
	}

	var bids []types.QueryBidResponse
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset

	walkErr := k.bids.Walk(ctx, r, func(_ keys.BidPrimaryKey, bid types.Bid) (bool, error) {
		if !stateSet[bid.State] {
			return false, nil
		}

		if !req.Filters.Accept(bid, bid.State) {
			return false, nil
		}

		if offset > 0 {
			offset--
			return false, nil
		}

		if req.Pagination.Limit == 0 {
			npk := keys.BidIDToKey(bid.ID)
			pkBuf := make([]byte, k.bids.KeyCodec().Size(npk))
			if _, err := k.bids.KeyCodec().Encode(pkBuf, npk); err != nil {
				return true, err
			}
			var err error
			nextKey, err = query.EncodePaginationKey(states, []byte{states[0]}, pkBuf, []byte{2})
			if err != nil {
				return true, err
			}
			return true, nil
		}

		acct, err := k.ekeeper.GetAccount(ctx, bid.ID.ToEscrowAccountID())
		if err != nil {
			return true, fmt.Errorf("%w: fetching escrow account for BidID=%s", err, bid.ID)
		}

		bids = append(bids, types.QueryBidResponse{
			Bid:           bid,
			EscrowAccount: acct,
		})
		req.Pagination.Limit--
		total++
		return false, nil
	})
	if walkErr != nil {
		return nil, status.Error(codes.Internal, walkErr.Error())
	}

	return &types.QueryBidsResponse{
		Bids: bids,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

// bidsProviderPath iterates bids via the Provider index.
func (k Querier) bidsProviderPath(
	ctx sdk.Context,
	req *types.QueryBidsRequest,
	states []byte,
	provider string,
	resumePK *keys.BidPrimaryKey,
) (*types.QueryBidsResponse, error) {
	stateSet := make(map[types.Bid_State]bool, len(states))
	for _, s := range states {
		stateSet[types.Bid_State(s)] = true
	}

	var iter indexes.MultiIterator[string, keys.BidPrimaryKey]
	var err error

	if resumePK != nil {
		if req.Pagination.Reverse {
			r := collections.NewPrefixedPairRange[string, keys.BidPrimaryKey](provider).EndInclusive(*resumePK).Descending()
			iter, err = k.bids.Indexes.Provider.Iterate(ctx, r)
		} else {
			r := collections.NewPrefixedPairRange[string, keys.BidPrimaryKey](provider).StartInclusive(*resumePK)
			iter, err = k.bids.Indexes.Provider.Iterate(ctx, r)
		}
	} else if req.Pagination.Reverse {
		iter, err = k.bids.Indexes.Provider.Iterate(ctx,
			collections.NewPrefixedPairRange[string, keys.BidPrimaryKey](provider).Descending())
	} else {
		iter, err = k.bids.Indexes.Provider.MatchExact(ctx, provider)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var bids []types.QueryBidResponse
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

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
			pk := keys.BidIDToKey(bid.ID)
			pkBuf := make([]byte, k.bids.KeyCodec().Size(pk))
			if _, encErr := k.bids.KeyCodec().Encode(pkBuf, pk); encErr != nil {
				scanErr = encErr
				return true
			}
			var encErr error
			nextKey, encErr = query.EncodePaginationKey(states, []byte{states[0]}, pkBuf, []byte{1})
			if encErr != nil {
				scanErr = encErr
			}
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
		total++
		return false
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if scanErr != nil {
		return nil, status.Error(codes.Internal, scanErr.Error())
	}

	return &types.QueryBidsResponse{
		Bids: bids,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

// bidsStatePath iterates bids via the State index.
func (k Querier) bidsStatePath(
	ctx sdk.Context,
	req *types.QueryBidsRequest,
	states []byte,
	resumePK *keys.BidPrimaryKey,
) (*types.QueryBidsResponse, error) {
	var bids []types.QueryBidResponse
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

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
				pk := keys.BidIDToKey(bid.ID)
				pkBuf := make([]byte, k.bids.KeyCodec().Size(pk))
				if _, encErr := k.bids.KeyCodec().Encode(pkBuf, pk); encErr != nil {
					scanErr = encErr
					return true
				}
				var encErr error
				nextKey, encErr = query.EncodePaginationKey(states[idx:], []byte{states[idx]}, pkBuf, []byte{0})
				if encErr != nil {
					scanErr = encErr
				}
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

	// Step 1: Resolve states, resumePK, and iteration path
	// pathType: 0=state-index, 1=provider-index, 2=owner-prefix
	states := make([]byte, 0, 3)
	var resumePK *keys.LeasePrimaryKey
	pathType := byte(0)
	var owner string
	var provider string

	if len(req.Pagination.Key) > 0 {
		// RESUME — all filters ignored, key provides everything
		var pkBytes, unsolicited []byte
		var err error
		states, _, pkBytes, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if len(unsolicited) != 1 {
			return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
		}

		pathType = unsolicited[0]

		_, pk, err := k.leases.KeyCodec().Decode(pkBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resumePK = &pk

		switch pathType {
		case 1: // provider path — recover provider from PK
			provider = resumePK.K2().K1()
		case 2: // owner path — recover owner from PK
			owner = resumePK.K1().K1()
		}
	} else {
		// INITIAL — resolve states from Filters.State
		if req.Filters.State != "" {
			stateVal := v1.Lease_State(v1.Lease_State_value[req.Filters.State])
			if stateVal == v1.LeaseStateInvalid {
				return nil, status.Error(codes.InvalidArgument, "invalid state value")
			}
			states = append(states, byte(stateVal))
		} else {
			states = append(states, byte(v1.LeaseActive), byte(v1.LeaseInsufficientFunds), byte(v1.LeaseClosed))
		}

		// Resolve iteration path from filters
		if req.Filters.Owner != "" {
			pathType = 2
			owner = req.Filters.Owner
		} else if req.Filters.Provider != "" {
			pathType = 1
			provider = req.Filters.Provider
		}
	}

	// Step 2: Owner path — iterate primary map with owner prefix
	if pathType == 2 {
		return k.leasesOwnerPath(ctx, req, states, owner, resumePK)
	}

	// Step 3: Provider path — iterate provider index
	if pathType == 1 {
		return k.leasesProviderPath(ctx, req, states, provider, resumePK)
	}

	// Step 4: State-index path
	if len(req.Pagination.Key) == 0 && req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	return k.leasesStatePath(ctx, req, states, resumePK)
}

// leasesOwnerPath iterates the primary map with an owner prefix.
func (k Querier) leasesOwnerPath(
	ctx sdk.Context,
	req *types.QueryLeasesRequest,
	states []byte,
	owner string,
	resumePK *keys.LeasePrimaryKey,
) (*types.QueryLeasesResponse, error) {
	stateSet := make(map[v1.Lease_State]bool, len(states))
	for _, s := range states {
		stateSet[v1.Lease_State(s)] = true
	}

	orderPrefix := collections.QuadPrefix[string, uint64, uint32, uint32](owner)
	leasePrefix := collections.PairPrefix[keys.OrderPrimaryKey, keys.ProviderPartKey](orderPrefix)
	r := new(collections.Range[keys.LeasePrimaryKey]).Prefix(leasePrefix)
	if resumePK != nil {
		if req.Pagination.Reverse {
			r.EndInclusive(*resumePK).Descending()
		} else {
			r.StartInclusive(*resumePK)
		}
	} else if req.Pagination.Reverse {
		r.Descending()
	}

	var leases []types.QueryLeaseResponse
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset

	walkErr := k.leases.Walk(ctx, r, func(_ keys.LeasePrimaryKey, lease v1.Lease) (bool, error) {
		if !stateSet[lease.State] {
			return false, nil
		}

		if !req.Filters.Accept(lease, lease.State) {
			return false, nil
		}

		if offset > 0 {
			offset--
			return false, nil
		}

		if req.Pagination.Limit == 0 {
			npk := keys.LeaseIDToKey(lease.ID)
			pkBuf := make([]byte, k.leases.KeyCodec().Size(npk))
			if _, err := k.leases.KeyCodec().Encode(pkBuf, npk); err != nil {
				return true, err
			}
			var err error
			nextKey, err = query.EncodePaginationKey(states, []byte{states[0]}, pkBuf, []byte{2})
			if err != nil {
				return true, err
			}
			return true, nil
		}

		payment, err := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
		if err != nil {
			return true, fmt.Errorf("%w: fetching escrow payment for LeaseID=%s", err, lease.ID)
		}

		leases = append(leases, types.QueryLeaseResponse{
			Lease:         lease,
			EscrowPayment: payment,
		})
		req.Pagination.Limit--
		total++
		return false, nil
	})
	if walkErr != nil {
		return nil, status.Error(codes.Internal, walkErr.Error())
	}

	return &types.QueryLeasesResponse{
		Leases: leases,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

// leasesProviderPath iterates leases via the Provider index.
func (k Querier) leasesProviderPath(
	ctx sdk.Context,
	req *types.QueryLeasesRequest,
	states []byte,
	provider string,
	resumePK *keys.LeasePrimaryKey,
) (*types.QueryLeasesResponse, error) {
	stateSet := make(map[v1.Lease_State]bool, len(states))
	for _, s := range states {
		stateSet[v1.Lease_State(s)] = true
	}

	var iter indexes.MultiIterator[string, keys.LeasePrimaryKey]
	var err error

	if resumePK != nil {
		if req.Pagination.Reverse {
			r := collections.NewPrefixedPairRange[string, keys.LeasePrimaryKey](provider).EndInclusive(*resumePK).Descending()
			iter, err = k.leases.Indexes.Provider.Iterate(ctx, r)
		} else {
			r := collections.NewPrefixedPairRange[string, keys.LeasePrimaryKey](provider).StartInclusive(*resumePK)
			iter, err = k.leases.Indexes.Provider.Iterate(ctx, r)
		}
	} else if req.Pagination.Reverse {
		iter, err = k.leases.Indexes.Provider.Iterate(ctx,
			collections.NewPrefixedPairRange[string, keys.LeasePrimaryKey](provider).Descending())
	} else {
		iter, err = k.leases.Indexes.Provider.MatchExact(ctx, provider)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var leases []types.QueryLeaseResponse
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

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
			pk := keys.LeaseIDToKey(lease.ID)
			pkBuf := make([]byte, k.leases.KeyCodec().Size(pk))
			if _, encErr := k.leases.KeyCodec().Encode(pkBuf, pk); encErr != nil {
				scanErr = encErr
				return true
			}
			var encErr error
			nextKey, encErr = query.EncodePaginationKey(states, []byte{states[0]}, pkBuf, []byte{1})
			if encErr != nil {
				scanErr = encErr
			}
			return true
		}

		payment, pmntErr := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
		if pmntErr != nil {
			scanErr = fmt.Errorf("%w: fetching escrow payment for LeaseID=%s", pmntErr, lease.ID)
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

	return &types.QueryLeasesResponse{
		Leases: leases,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

// leasesStatePath iterates leases via the State index.
func (k Querier) leasesStatePath(
	ctx sdk.Context,
	req *types.QueryLeasesRequest,
	states []byte,
	resumePK *keys.LeasePrimaryKey,
) (*types.QueryLeasesResponse, error) {
	var leases []types.QueryLeaseResponse
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

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
				pk := keys.LeaseIDToKey(lease.ID)
				pkBuf := make([]byte, k.leases.KeyCodec().Size(pk))
				if _, encErr := k.leases.KeyCodec().Encode(pkBuf, pk); encErr != nil {
					scanErr = encErr
					return true
				}
				var encErr error
				nextKey, encErr = query.EncodePaginationKey(states[idx:], []byte{states[idx]}, pkBuf, []byte{0})
				if encErr != nil {
					scanErr = encErr
				}
				return true
			}

			payment, pmntErr := k.ekeeper.GetPayment(ctx, lease.ID.ToEscrowPaymentID())
			if pmntErr != nil {
				scanErr = fmt.Errorf("%w: fetching escrow payment for LeaseID=%s", pmntErr, lease.ID)
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
