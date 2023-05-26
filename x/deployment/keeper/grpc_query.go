package keeper

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type Querier struct {
	Keeper
}

var _ types.QueryServer = Querier{}

// Deployments returns deployments based on filters
func (k Querier) Deployments(c context.Context, req *types.QueryDeploymentsRequest) (*types.QueryDeploymentsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	stateVal := types.Deployment_State(types.Deployment_State_value[req.Filters.State])

	if req.Filters.State != "" && stateVal == types.DeploymentStateInvalid {
		return nil, status.Error(codes.InvalidArgument, "invalid state value")
	}

	var deployments types.DeploymentResponses
	ctx := sdk.UnwrapSDKContext(c)

	searchPrefix, err := deploymentPrefixFromFilter(req.Filters)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	depStore := prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

	pageRes, err := sdkquery.FilteredPaginate(depStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
		var deployment types.Deployment

		err := k.cdc.Unmarshal(value, &deployment)
		if err != nil {
			return false, err
		}

		// filter deployments with provided filters
		if req.Filters.Accept(deployment, stateVal) {
			if accumulate {

				account, err := k.ekeeper.GetAccount(
					ctx,
					types.EscrowAccountForDeployment(deployment.ID()),
				)
				if err != nil {
					return true, fmt.Errorf("%w: fetching escrow account for DeploymentID=%s", err, deployment.DeploymentID)
				}

				value := types.QueryDeploymentResponse{
					Deployment:    deployment,
					Groups:        k.GetGroups(ctx, deployment.ID()),
					EscrowAccount: account,
				}

				deployments = append(deployments, value)
			}

			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryDeploymentsResponse{
		Deployments: deployments,
		Pagination:  pageRes,
	}, nil
}

// Deployment returns deployment details based on DeploymentID
func (k Querier) Deployment(c context.Context, req *types.QueryDeploymentRequest) (*types.QueryDeploymentResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if _, err := sdk.AccAddressFromBech32(req.ID.Owner); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner address")
	}

	ctx := sdk.UnwrapSDKContext(c)

	deployment, found := k.GetDeployment(ctx, req.ID)
	if !found {
		return nil, types.ErrDeploymentNotFound
	}

	account, err := k.ekeeper.GetAccount(
		ctx,
		types.EscrowAccountForDeployment(req.ID),
	)
	if err != nil {
		return &types.QueryDeploymentResponse{}, err
	}

	value := &types.QueryDeploymentResponse{
		Deployment:    deployment,
		Groups:        k.GetGroups(ctx, req.ID),
		EscrowAccount: account,
	}

	return value, nil
}

// Group returns group details based on GroupID
func (k Querier) Group(c context.Context, req *types.QueryGroupRequest) (*types.QueryGroupResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if _, err := sdk.AccAddressFromBech32(req.ID.Owner); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner address")
	}

	ctx := sdk.UnwrapSDKContext(c)

	group, found := k.GetGroup(ctx, req.ID)
	if !found {
		return nil, types.ErrGroupNotFound
	}

	return &types.QueryGroupResponse{Group: group}, nil
}
