package keeper

import (
	"bytes"
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "pkg.akt.dev/go/node/escrow/types/v1"
	etypes "pkg.akt.dev/go/node/escrow/v1"

	"pkg.akt.dev/node/v2/util/query"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	*keeper
}

var _ etypes.QueryServer = Querier{}

func (k Querier) Accounts(c context.Context, req *etypes.QueryAccountsRequest) (*etypes.QueryAccountsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	states := make([]byte, 0, 3)
	var searchPrefix []byte
	var startKey []byte

	// nolint: gocritic
	if len(req.Pagination.Key) > 0 {
		var innerKey []byte
		var err error
		states, searchPrefix, innerKey, _, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		// Validate the inner key by parsing the reconstructed full store key
		if _, _, err = ParseAccountKey(append(bytes.Clone(searchPrefix), innerKey...)); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		// Safe copy -- iterator may hold a reference to the start key
		startKey = make([]byte, len(innerKey))
		copy(startKey, innerKey)
	} else if req.State != "" {
		stateVal := types.State(types.State_value[req.State])

		if req.State != "" && stateVal == types.StateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		states = append(states, byte(types.StateOpen), byte(types.StateClosed), byte(types.StateOverdrawn))
	}

	var accounts types.Accounts
	var nextKey []byte
	total := uint64(0)

	iters := make([]storetypes.Iterator, 0, len(states))
	defer func() {
		for _, it := range iters {
			_ = it.Close()
		}
	}()

	var idx int

	for idx = range states {
		state := types.State(states[idx])

		if idx > 0 {
			startKey = nil
		}

		if startKey == nil {
			req.State = state.String()
			searchPrefix = BuildSearchPrefix(AccountPrefix, req.State, req.XID)
		}

		searchStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)
		iter := searchStore.Iterator(startKey, nil)
		iters = append(iters, iter)

		count := uint64(0)

		for ; iter.Valid() && req.Pagination.Limit > 0; iter.Next() {
			id, _, err := ParseAccountKey(append(searchPrefix, iter.Key()...))
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			acc := types.Account{ID: id}

			if err := k.cdc.Unmarshal(iter.Value(), &acc.State); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			accounts = append(accounts, acc)
			req.Pagination.Limit--
			count++
		}

		total += count

		// Page full and more items exist -- encode NextKey for continuation
		if iter.Valid() && req.Pagination.Limit == 0 {
			var err error
			nextKey, err = query.EncodePaginationKey(states[idx:], searchPrefix, iter.Key(), nil)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			break
		}
	}

	return &etypes.QueryAccountsResponse{
		Accounts: accounts,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

func (k Querier) Payments(c context.Context, req *etypes.QueryPaymentsRequest) (*etypes.QueryPaymentsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	states := make([]byte, 0, 3)
	var searchPrefix []byte
	var startKey []byte

	// nolint: gocritic
	if len(req.Pagination.Key) > 0 {
		var innerKey []byte
		var err error
		states, searchPrefix, innerKey, _, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		// Validate the inner key by parsing the reconstructed full store key
		if _, _, err = ParsePaymentKey(append(bytes.Clone(searchPrefix), innerKey...)); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		// Safe copy -- iterator may hold a reference to the start key
		startKey = make([]byte, len(innerKey))
		copy(startKey, innerKey)
	} else if req.State != "" {
		stateVal := types.State(types.State_value[req.State])

		if req.State != "" && stateVal == types.StateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		states = append(states, byte(types.StateOpen), byte(types.StateClosed), byte(types.StateOverdrawn))
	}

	var payments types.Payments
	var nextKey []byte
	total := uint64(0)

	iters := make([]storetypes.Iterator, 0, len(states))
	defer func() {
		for _, it := range iters {
			_ = it.Close()
		}
	}()

	var idx int

	for idx = range states {
		state := types.State(states[idx])

		if idx > 0 {
			startKey = nil
		}

		if startKey == nil {
			req.State = state.String()
			searchPrefix = BuildSearchPrefix(PaymentPrefix, req.State, req.XID)
		}

		searchStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)
		iter := searchStore.Iterator(startKey, nil)
		iters = append(iters, iter)

		count := uint64(0)

		for ; iter.Valid() && req.Pagination.Limit > 0; iter.Next() {
			id, _, err := ParsePaymentKey(append(searchPrefix, iter.Key()...))
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			pmnt := types.Payment{ID: id}

			if err := k.cdc.Unmarshal(iter.Value(), &pmnt.State); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			payments = append(payments, pmnt)
			req.Pagination.Limit--
			count++
		}

		total += count

		// Page full and more items exist -- encode NextKey for continuation
		if iter.Valid() && req.Pagination.Limit == 0 {
			var err error
			nextKey, err = query.EncodePaginationKey(states[idx:], searchPrefix, iter.Key(), nil)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			break
		}
	}

	return &etypes.QueryPaymentsResponse{
		Payments: payments,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

func BuildSearchPrefix(prefix []byte, state string, xid string) []byte {
	buf := &bytes.Buffer{}

	buf.Write(prefix)
	if state != "" {
		st := types.State(types.State_value[state])
		buf.Write(stateToPrefix(st))
		if xid != "" {
			buf.WriteRune('/')
			buf.WriteString(xid)
		}
	}

	return buf.Bytes()
}
