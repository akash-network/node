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

func (k Querier) PriceFeedConfig(ctx context.Context, request *types.QueryPriceFeedConfigRequest) (*types.QueryPriceFeedConfigResponse, error) {
	//TODO implement me
	panic("implement me")
}

var _ types.QueryServer = Querier{}

func (k Querier) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := k.GetParams(sdkCtx)

	return &types.QueryParamsResponse{Params: params}, nil
}
