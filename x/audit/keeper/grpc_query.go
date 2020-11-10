package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ovrclk/akash/x/audit/types"
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

	var providers types.Providers
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.skey)

	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(key []byte, value []byte) error {
		var provider types.Provider

		err := q.cdc.UnmarshalBinaryBare(value, &provider)
		if err != nil {
			return err
		}

		providers = append(providers, provider)
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

func (q Querier) ProviderValidatorAttributes(
	c context.Context,
	req *types.QueryProviderValidatorRequest,
) (*types.QueryProvidersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	owner, err := sdk.AccAddressFromBech32(req.Owner)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	validator, err := sdk.AccAddressFromBech32(req.Validator)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	ctx := sdk.UnwrapSDKContext(c)

	provider, found := q.GetProviderByValidator(ctx, types.ProviderID{
		Owner:     owner,
		Validator: validator,
	})
	if !found {
		return nil, types.ErrProviderNotFound
	}

	return &types.QueryProvidersResponse{
		Providers:  types.Providers{provider},
		Pagination: nil,
	}, nil
}

func (q Querier) ValidatorAttributes(
	c context.Context,
	req *types.QueryValidatorAttributesRequest,
) (*types.QueryProvidersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	var providers types.Providers
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(q.skey)

	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(key []byte, value []byte) error {
		var provider types.Provider

		err := q.cdc.UnmarshalBinaryBare(value, &provider)
		if err != nil {
			return err
		}

		providers = append(providers, provider)
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
