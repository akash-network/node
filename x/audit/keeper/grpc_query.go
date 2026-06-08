package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "pkg.akt.dev/go/node/audit/v1"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	Keeper
}

var _ types.QueryServer = Querier{}

func (q Querier) AllProvidersAttributes(
	c context.Context,
	req *types.QueryAllProvidersAttributesRequest,
) (*types.QueryProvidersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	var providers types.AuditedProviders
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.skey)

	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(key []byte, value []byte) error {
		id := ParseIDFromKey(key)

		var sVal types.AuditedAttributesStore
		if err := q.cdc.Unmarshal(value, &sVal); err != nil {
			return err
		}

		providers = append(providers, types.AuditedProvider{
			Owner:      id.Owner.String(),
			Auditor:    id.Auditor.String(),
			Attributes: sVal.Attributes,
		})
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryProvidersResponse{
		Providers:  providers,
		Pagination: pageRes,
	}, nil
}

func (q Querier) ProviderAttributes(
	c context.Context,
	req *types.QueryProviderAttributesRequest,
) (*types.QueryProvidersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	owner, err := sdk.AccAddressFromBech32(req.Owner)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	ctx := sdk.UnwrapSDKContext(c)

	provider, found := q.GetProviderAttributes(ctx, owner)
	if !found {
		return nil, types.ErrProviderNotFound
	}

	return &types.QueryProvidersResponse{
		Providers:  provider,
		Pagination: nil,
	}, nil
}

func (q Querier) ProviderAuditorAttributes(
	c context.Context,
	req *types.QueryProviderAuditorRequest,
) (*types.QueryProvidersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	owner, err := sdk.AccAddressFromBech32(req.Owner)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	auditor, err := sdk.AccAddressFromBech32(req.Auditor)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	ctx := sdk.UnwrapSDKContext(c)

	provider, found := q.GetProviderByAuditor(ctx, types.ProviderID{
		Owner:   owner,
		Auditor: auditor,
	})
	if !found {
		return nil, types.ErrProviderNotFound
	}

	return &types.QueryProvidersResponse{
		Providers:  types.AuditedProviders{provider},
		Pagination: nil,
	}, nil
}

func (q Querier) AuditorAttributes(
	c context.Context,
	req *types.QueryAuditorAttributesRequest,
) (*types.QueryProvidersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	auditor, err := sdk.AccAddressFromBech32(req.Auditor)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	var providers types.AuditedProviders
	ctx := sdk.UnwrapSDKContext(c)
	store := ctx.KVStore(q.skey)

	pageRes, err := sdkquery.FilteredPaginate(store, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		id := ParseIDFromKey(key)
		if !id.Auditor.Equals(auditor) {
			return false, nil
		}
		if accumulate {
			var sVal types.AuditedAttributesStore
			if err := q.cdc.Unmarshal(value, &sVal); err != nil {
				return false, err
			}
			providers = append(providers, types.AuditedProvider{
				Owner:      id.Owner.String(),
				Auditor:    id.Auditor.String(),
				Attributes: sVal.Attributes,
			})
		}
		return true, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryProvidersResponse{Providers: providers, Pagination: pageRes}, nil
}
