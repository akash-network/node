package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"
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

	stateVal := types.Certificate_State(types.Certificate_State_value[req.Filter.State])

	if req.Filter.State != "" && stateVal == types.CertificateStateInvalid {
		return nil, status.Error(codes.InvalidArgument, "invalid state value")
	}

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filter.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	states := make([]types.Certificate_State, 0, 2)

	// setup for case 3 - cross-index search
	if req.Filter.State == "" {
		// request has pagination key set, determine store prefix
		if len(req.Pagination.Key) > 0 {
			if len(req.Pagination.Key) < 3 {
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}

			switch req.Pagination.Key[2] {
			case CertStateValidPrefixID:
				states = append(states, types.CertificateValid)
				fallthrough
			case CertStateRevokedPrefixID:
				states = append(states, types.CertificateRevoked)
			default:
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}
		} else {
			// request does not have pagination set. Start from valid store
			states = append(states, types.CertificateValid)
			states = append(states, types.CertificateRevoked)
		}
	} else {
		states = append(states, stateVal)
	}

	var certificates types.CertificatesResponse
	var pageRes *sdkquery.PageResponse

	total := uint64(0)

	for _, state := range states {
		var searchPrefix []byte
		var err error

		req.Filter.State = state.String()

		searchPrefix, err = filterToPrefix(req.Filter)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
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

				// return true, nil
			}

			return true, nil
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

	return &types.QueryCertificatesResponse{
		Certificates: certificates,
		Pagination:   pageRes,
	}, nil
}
