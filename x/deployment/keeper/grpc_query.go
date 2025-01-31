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
	ctx := sdk.UnwrapSDKContext(c)

	defer func() {
		if r := recover(); r != nil {
			ctx.Logger().Error(fmt.Sprintf("%v", r))
		}
	}()
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	stateVal := types.Deployment_State(types.Deployment_State_value[req.Filters.State])

	if req.Filters.State != "" && stateVal == types.DeploymentStateInvalid {
		return nil, status.Error(codes.InvalidArgument, "invalid state value")
	}

	var store prefix.Store

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination != nil && req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	states := make([]types.Deployment_State, 0, 2)

	// setup for case 3 - cross-index search
	if req.Filters.State == "" {
		// request has pagination key set, determine store prefix
		if len(req.Pagination.Key) > 0 {
			if len(req.Pagination.Key) < 3 {
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}

			switch req.Pagination.Key[2] {
			case DeploymentStateActivePrefixID:
				states = append(states, types.DeploymentActive)
				fallthrough
			case DeploymentStateClosedPrefixID:
				states = append(states, types.DeploymentClosed)
			default:
				return nil, status.Error(codes.InvalidArgument, "invalid pagination key")
			}
		} else {
			// request does not have pagination set. Start from active store
			states = append(states, types.DeploymentActive)
			states = append(states, types.DeploymentClosed)
		}
	} else {
		states = append(states, stateVal)
	}

	var deployments types.DeploymentResponses
	var pageRes *sdkquery.PageResponse

	for _, state := range states {
		var searchPrefix []byte
		var err error

		req.Filters.State = state.String()

		searchPrefix, err = deploymentPrefixFromFilter(req.Filters)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		store = prefix.NewStore(ctx.KVStore(k.skey), searchPrefix)

		count := uint64(0)

		pageRes, err = sdkquery.FilteredPaginate(store, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
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
					count++
				}

				return true, nil
			}

			return false, nil
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		req.Pagination.Limit -= count

		if req.Pagination.Limit == 0 {
			break
		}
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
