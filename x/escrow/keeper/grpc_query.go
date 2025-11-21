package keeper

import (
	"bytes"
	"context"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/node/escrow/v1"

	"pkg.akt.dev/node/util/query"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	*keeper
}

var _ v1.QueryServer = Querier{}

func (k Querier) Accounts(c context.Context, req *v1.QueryAccountsRequest) (*v1.QueryAccountsResponse, error) {
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
	} else if req.State != "" {
		stateVal := types.State(types.State_value[req.State])

		if req.State != "" && stateVal == types.StateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		// request does not have a pagination set. Start from active store
		states = append(states, []byte{byte(types.StateOpen), byte(types.StateClosed), byte(types.StateOverdrawn)}...)
	}

	var accounts types.Accounts
	var pageRes *sdkquery.PageResponse

	total := uint64(0)

	for idx := range states {
		state := types.State(states[idx])

		var err error
		if idx > 0 {
			req.Pagination.Key = nil
		}

		if len(req.Pagination.Key) == 0 {
			req.State = state.String()

			searchPrefix = BuildSearchPrefix(AccountPrefix, req.State, req.XID)
		}

		searchStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

		count := uint64(0)

		pageRes, err = sdkquery.FilteredPaginate(searchStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
			id, _ := ParseAccountKey(append(searchPrefix, key...))
			acc := types.Account{
				ID: id,
			}

			er := k.cdc.Unmarshal(value, &acc.State)
			if er != nil {
				return false, er
			}

			accounts = append(accounts, acc)
			count++

			return false, nil
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		req.Pagination.Limit -= count
		total += count

		if req.Pagination.Limit == 0 {
			if len(pageRes.NextKey) > 0 {
				pageRes.NextKey, err = query.EncodePaginationKey(states[idx:], searchPrefix, pageRes.NextKey, nil)
				if err != nil {
					pageRes.Total = total
					return &v1.QueryAccountsResponse{
						Accounts:   accounts,
						Pagination: pageRes,
					}, status.Error(codes.Internal, err.Error())
				}
			}

			break
		}
	}

	pageRes.Total = total

	return &v1.QueryAccountsResponse{
		Accounts:   accounts,
		Pagination: pageRes,
	}, nil
}

func (k Querier) Payments(c context.Context, req *v1.QueryPaymentsRequest) (*v1.QueryPaymentsResponse, error) {
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
	} else if req.State != "" {
		stateVal := types.State(types.State_value[req.State])

		if req.State != "" && stateVal == types.StateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		// request does not have a pagination set. Start from active store
		states = append(states, []byte{byte(types.StateOpen), byte(types.StateClosed), byte(types.StateOverdrawn)}...)
	}

	var payments types.Payments
	var pageRes *sdkquery.PageResponse

	total := uint64(0)

	for idx := range states {
		state := types.State(states[idx])

		var err error
		if idx > 0 {
			req.Pagination.Key = nil
		}

		if len(req.Pagination.Key) == 0 {
			req.State = state.String()

			searchPrefix = BuildSearchPrefix(PaymentPrefix, req.State, req.XID)
		}

		searchStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

		count := uint64(0)

		pageRes, err = sdkquery.FilteredPaginate(searchStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
			id, _ := ParsePaymentKey(append(searchPrefix, key...))
			pmnt := types.Payment{
				ID: id,
			}

			er := k.cdc.Unmarshal(value, &pmnt.State)
			if er != nil {
				return false, er
			}

			payments = append(payments, pmnt)
			count++

			return false, nil
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		req.Pagination.Limit -= count
		total += count

		if req.Pagination.Limit == 0 {
			if len(pageRes.NextKey) > 0 {
				pageRes.NextKey, err = query.EncodePaginationKey(states[idx:], searchPrefix, pageRes.NextKey, nil)
				if err != nil {
					pageRes.Total = total
					return &v1.QueryPaymentsResponse{
						Payments:   payments,
						Pagination: pageRes,
					}, status.Error(codes.Internal, err.Error())
				}
			}

			break
		}
	}

	pageRes.Total = total

	return &v1.QueryPaymentsResponse{
		Payments:   payments,
		Pagination: pageRes,
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
