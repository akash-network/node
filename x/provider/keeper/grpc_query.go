package keeper

import (
	"context"

	"cosmossdk.io/store/prefix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "pkg.akt.dev/go/node/provider/v1beta4"
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

	store := prefix.NewStore(ctx.KVStore(k.skey), types.ProviderPrefix())

	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(_ []byte, value []byte) error {
		var provider types.Provider

		err := k.cdc.Unmarshal(value, &provider)
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

func (k Querier) ProviderMaintenance(c context.Context, req *types.QueryProviderMaintenanceRequest) (*types.QueryProviderMaintenanceResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, err := sdk.AccAddressFromBech32(req.Provider)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	ctx := sdk.UnwrapSDKContext(c)
	if _, found := k.Get(ctx, provider); !found {
		return nil, types.ErrProviderNotFound
	}

	record, found := k.GetMaintenance(ctx, req.MaintenanceId)
	if !found || record.Provider != req.Provider {
		return nil, status.Error(codes.NotFound, "maintenance record not found")
	}

	return &types.QueryProviderMaintenanceResponse{
		Maintenance: types.ProviderMaintenanceWithStatus{
			Record: record,
			Status: MaintenanceStatus(ctx.BlockTime(), record),
		},
	}, nil
}

func (k Querier) ProviderMaintenances(c context.Context, req *types.QueryProviderMaintenancesRequest) (*types.QueryProviderMaintenancesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, err := sdk.AccAddressFromBech32(req.Provider)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	ctx := sdk.UnwrapSDKContext(c)
	if _, found := k.Get(ctx, provider); !found {
		return nil, types.ErrProviderNotFound
	}

	var records []types.ProviderMaintenanceWithStatus
	store := prefix.NewStore(ctx.KVStore(k.skey), ProviderMaintenanceOwnerPrefix(provider))

	pageRes, err := sdkquery.FilteredPaginate(store, req.Pagination, func(key []byte, _ []byte, accumulate bool) (bool, error) {
		if len(key) != 8 {
			return false, nil
		}

		record, found := k.GetMaintenance(ctx, uint64FromBytes(key))
		if !found {
			return false, nil
		}

		status := MaintenanceStatus(ctx.BlockTime(), record)
		if req.StatusFilter != types.ProviderMaintenanceStatus_provider_maintenance_status_unspecified &&
			status != req.StatusFilter {
			return false, nil
		}

		if accumulate {
			records = append(records, types.ProviderMaintenanceWithStatus{
				Record: record,
				Status: status,
			})
		}

		return true, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryProviderMaintenancesResponse{
		Maintenance: records,
		Pagination:  pageRes,
	}, nil
}

func (k Querier) Params(c context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	ctx := sdk.UnwrapSDKContext(c)

	return &types.QueryParamsResponse{Params: k.GetParams(ctx)}, nil
}

func (k Querier) Registration(c context.Context, req *types.QueryRegistrationRequest) (*types.QueryRegistrationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, err := sdk.AccAddressFromBech32(req.Provider)
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	ctx := sdk.UnwrapSDKContext(c)
	registration, found := k.GetRegistration(ctx, provider)
	if !found {
		return nil, types.ErrProviderNotFound
	}

	return &types.QueryRegistrationResponse{Registration: registration}, nil
}
