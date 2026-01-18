package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/oracle/v1"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	Keeper
}

func (k Querier) Prices(ctx context.Context, req *types.QueryPricesRequest) (*types.QueryPricesResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sctx := sdk.UnwrapSDKContext(ctx)
	keeper := k.Keeper.(*keeper)

	var err error
	var prices []types.PriceData
	var pageRes *query.PageResponse

	filters := req.Filters

	// Query specific price data based on filters
	if filters.Height > 0 {
		// Query specific height
		err = keeper.latestPrices.Walk(sctx, nil, func(key types.PriceDataID, height int64) (bool, error) {
			if (filters.AssetDenom == "" || key.Denom == filters.AssetDenom) &&
				(filters.BaseDenom == "" || key.BaseDenom == filters.BaseDenom) {

				recordID := types.PriceDataRecordID{
					Source:    key.Source,
					Denom:     key.Denom,
					BaseDenom: key.BaseDenom,
					Height:    filters.Height,
				}

				state, err := keeper.prices.Get(sctx, recordID)
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
		pageReq := &query.PageRequest{}
		if req.Pagination != nil {
			*pageReq = *req.Pagination
		}
		pageReq.Reverse = true

		prices, pageRes, err = query.CollectionFilteredPaginate(
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
	}

	return &types.QueryPricesResponse{
		Prices:     prices,
		Pagination: pageRes,
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
