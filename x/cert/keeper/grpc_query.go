package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "pkg.akt.dev/go/node/cert/v1"

	"pkg.akt.dev/node/util/query"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type querier struct {
	keeper
}

var _ types.QueryServer = &querier{}

func (q querier) Certificates(c context.Context, req *types.QueryCertificatesRequest) (*types.QueryCertificatesResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filter.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	states := make([]byte, 0, 2)
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
	} else if req.Filter.State != "" {
		stateVal := types.State(types.State_value[req.Filter.State])

		if req.Filter.State != "" && stateVal == types.CertificateStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		states = append(states, byte(stateVal))
	} else {
		// request does not have pagination set. Start from valid store
		states = append(states, byte(types.CertificateValid))
		states = append(states, byte(types.CertificateRevoked))
	}

	var certificates types.CertificatesResponse
	var pageRes *sdkquery.PageResponse

	total := uint64(0)

	for idx := range states {
		state := types.State(states[idx])
		var err error

		if idx > 0 {
			req.Pagination.Key = nil
		}

		if len(req.Pagination.Key) == 0 {
			req.Filter.State = state.String()

			searchPrefix, err = filterToPrefix(req.Filter)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

		searchStore := prefix.NewStore(ctx.KVStore(q.skey), searchPrefix)

		count := uint64(0)

		pageRes, err = sdkquery.FilteredPaginate(searchStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
			if accumulate {
				// we need serial number for cert id which can be obtained from either key or by parsing actual cert
				// latter is way slower so we extract it from the key.
				// As key provided in the FilteredPaginate callback does not include the prefix, we complete it
				// by prepending the search prefix so ParseCertKey can work properly
				fKey := make([]byte, len(searchPrefix)+len(key))
				copy(fKey, searchPrefix)
				copy(fKey[len(searchPrefix):], key)

				_, item, err := q.unmarshal(fKey, value)
				if err != nil {
					return false, err
				}

				certificates = append(certificates, item)
				count++
			}

			return true, nil
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

					return &types.QueryCertificatesResponse{
						Certificates: certificates,
						Pagination:   pageRes,
					}, status.Error(codes.Internal, err.Error())
				}
			}

			break
		}
	}

	pageRes.Total = total

	return &types.QueryCertificatesResponse{
		Certificates: certificates,
		Pagination:   pageRes,
	}, nil
}
