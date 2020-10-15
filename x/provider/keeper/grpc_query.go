package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/ovrclk/akash/x/provider/types"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	Keeper
}

var _ types.QueryServer = Querier{}

// Providers returns providers list
func (k Querier) Providers(c context.Context, req *types.QueryProvidersRequest) (*types.QueryProvidersResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	var providers types.Providers
	ctx := sdk.UnwrapSDKContext(c)

	store := ctx.KVStore(k.skey)

	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(key []byte, value []byte) error {
		var provider types.Provider

		err := k.cdc.UnmarshalBinaryBare(value, &provider)
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

// Provider returns provider details based on owner address
func (k Querier) Provider(c context.Context, req *types.QueryProviderRequest) (*types.QueryProviderResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	owner, err := sdk.AccAddressFromBech32(req.Owner)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	ctx := sdk.UnwrapSDKContext(c)

	provider, found := k.Get(ctx, owner)
	if !found {
		return nil, types.ErrProviderNotFound
	}

	return &types.QueryProviderResponse{Provider: provider}, nil
}
