package query

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

var _ types.QueryServer = keeper.Keeper{}

// Deployment returns deployment details based on DeploymentID
func (q keeper.Keeper) Deployment(c context.Context, req *types.QueryDeploymentRequest) (*types.QueryDeploymentResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.ID.Owner.Empty() {
		return nil, status.Error(codes.InvalidArgument, "owner cannot be empty")
	}

	ctx := sdk.UnwrapSDKContext(c)

	deployment, found := q.GetDeployment(ctx,req.ID)
	if !found {
		return nil,types.ErrDeploymentNotFound
	}

	value := types.DeploymentResponse{
		Deployment: deployment,
		Groups:     q.GetGroups(ctx, req.ID),
	}

	return &types.QueryDeploymentResponse{Deployment: value}, nil
}
