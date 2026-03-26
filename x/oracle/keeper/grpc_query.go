package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	Keeper
}

func (k Querier) Prices(ctx context.Context, req *types.QueryPricesRequest) (*types.QueryPricesResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	keeper := k.Keeper.(*keeper)

	filters := req.Filters

	pageReq := &query.PageRequest{}
	if req.Pagination != nil {
		*pageReq = *req.Pagination
	}

	prices, pageRes, err := query.CollectionFilteredPaginate(
		ctx,
		keeper.prices,
		pageReq,
		func(key types.PriceDataRecordID, _ types.PriceDataState) (bool, error) {
			if filters.AssetDenom != "" && key.Denom != filters.AssetDenom {
				return false, nil
			}
			if filters.BaseDenom != "" && key.BaseDenom != filters.BaseDenom {
				return false, nil
			}
			if !filters.StartTime.IsZero() && key.Timestamp.Before(filters.StartTime) {
				return false, nil
			}
			if !filters.EndTime.IsZero() && key.Timestamp.After(filters.EndTime) {
				return false, nil
			}
			return true, nil
		},
		func(key types.PriceDataRecordID, val types.PriceDataState) (types.PriceData, error) {
			return types.PriceData{
				ID:    key,
				State: val,
			}, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &types.QueryPricesResponse{
		Prices:     prices,
		Pagination: pageRes,
	}, nil
}

func (k Querier) AggregatedPrice(ctx context.Context, req *types.QueryAggregatedPriceRequest) (*types.QueryAggregatedPriceResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sctx := sdk.UnwrapSDKContext(ctx)
	keeper := k.Keeper.(*keeper)

	aggregatedPrice, err := keeper.getAggregatedPrice(sctx, req.Denom)
	if err != nil {
		return nil, err
	}

	priceHealth, err := keeper.pricesHealth.Get(sctx, types.DataID{
		Denom:     req.Denom,
		BaseDenom: sdkutil.DenomUSD,
	})
	if err != nil {
		return nil, err
	}

	return &types.QueryAggregatedPriceResponse{
		AggregatedPrice: aggregatedPrice,
		PriceHealth:     priceHealth,
	}, nil
}

var _ types.QueryServer = Querier{}

func (k Querier) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := k.GetParams(sdkCtx)

	if err != nil {
		return nil, err
	}

	return &types.QueryParamsResponse{Params: params}, nil
}
