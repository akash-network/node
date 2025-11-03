package keeper

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/epochs/v1beta1"
)

var _ types.QueryServer = Querier{}

// Querier defines a wrapper around the x/epochs keeper providing gRPC method
// handlers.
type Querier struct {
	Keeper
}

// NewQuerier initializes new querier.
func NewQuerier(k Keeper) Querier {
	return Querier{Keeper: k}
}

// EpochInfos provide running epochInfos.
func (q Querier) EpochInfos(ctx context.Context, _ *types.QueryEpochInfosRequest) (*types.QueryEpochInfosResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	allEpochs := make([]types.EpochInfo, 0)
	err := q.IterateEpochs(sctx, func(_ string, info types.EpochInfo) (bool, error) {
		allEpochs = append(allEpochs, info)
		return false, nil
	})

	return &types.QueryEpochInfosResponse{
		Epochs: allEpochs,
	}, err
}

// CurrentEpoch provides current epoch of specified identifier.
func (q Querier) CurrentEpoch(ctx context.Context, req *types.QueryCurrentEpochRequest) (*types.QueryCurrentEpochResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is empty")
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	info, err := q.GetEpoch(sctx, req.Identifier)
	if err != nil {
		return nil, errors.New("not available identifier")
	}

	return &types.QueryCurrentEpochResponse{
		CurrentEpoch: info.CurrentEpoch,
	}, nil
}
