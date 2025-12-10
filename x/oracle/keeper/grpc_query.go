package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/oracle/v1"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	Keeper
}

func (k Querier) Prices(ctx context.Context, request *types.QueryPricesRequest) (*types.QueryPricesResponse, error) {
	if request == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	keeper := k.Keeper.(*keeper)

	var prices []types.PriceData

	// Build range based on filters
	filters := request.Filters

	// If no specific filters, return aggregated prices
	if filters.AssetDenom == "" && filters.BaseDenom == "" {
		// Return empty for now - could implement returning all prices
		return &types.QueryPricesResponse{
			Prices:     prices,
			Pagination: nil,
		}, nil
	}

	// Query specific price data based on filters
	if filters.Height > 0 {
		// Query specific height
		err := keeper.latestPrices.Walk(sdkCtx, nil, func(key types.PriceDataID, height int64) (bool, error) {
			if (filters.AssetDenom == "" || key.Denom == filters.AssetDenom) &&
				(filters.BaseDenom == "" || key.BaseDenom == filters.BaseDenom) {

				recordID := types.PriceDataRecordID{
					Source:    key.Source,
					Denom:     key.Denom,
					BaseDenom: key.BaseDenom,
					Height:    filters.Height,
				}

				state, err := keeper.prices.Get(sdkCtx, recordID)
				if err == nil {
					prices = append(prices, types.PriceData{
						ID:    recordID,
						State: state,
					})
				}
			}
			return false, nil
		})

		if err != nil {
			return nil, err
		}
	} else {
		// Query latest prices
		err := keeper.latestPrices.Walk(sdkCtx, nil, func(key types.PriceDataID, height int64) (bool, error) {
			if (filters.AssetDenom == "" || key.Denom == filters.AssetDenom) &&
				(filters.BaseDenom == "" || key.BaseDenom == filters.BaseDenom) {

				recordID := types.PriceDataRecordID{
					Source:    key.Source,
					Denom:     key.Denom,
					BaseDenom: key.BaseDenom,
					Height:    height,
				}

				state, err := keeper.prices.Get(sdkCtx, recordID)
				if err == nil {
					prices = append(prices, types.PriceData{
						ID:    recordID,
						State: state,
					})
				}
			}
			return false, nil
		})

		if err != nil {
			return nil, err
		}
	}

	return &types.QueryPricesResponse{
		Prices:     prices,
		Pagination: nil,
	}, nil
}

func (k Querier) PriceFeedConfig(ctx context.Context, request *types.QueryPriceFeedConfigRequest) (*types.QueryPriceFeedConfigResponse, error) {
	if request == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	// For now, return a basic response indicating the config is not set up
	// This can be extended later when Pyth integration is added
	return &types.QueryPriceFeedConfigResponse{
		PriceFeedId:         "",
		PythContractAddress: "",
		Enabled:             false,
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
