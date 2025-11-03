package keeper

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/indexes"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	"pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/v2/x/deployment/keeper/keys"
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

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	if len(req.Pagination.Key) > 0 {
		return nil, status.Error(codes.InvalidArgument, "key-based pagination is not supported")
	}

	ctx := sdk.UnwrapSDKContext(c)

	// Determine which states to iterate
	states := []v1.Deployment_State{v1.DeploymentActive, v1.DeploymentClosed}
	if req.Filters.State != "" {
		stateVal := v1.Deployment_State(v1.Deployment_State_value[req.Filters.State])
		if stateVal == v1.DeploymentStateInvalid {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}
		states = []v1.Deployment_State{stateVal}
	}

	if req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	var deployments types.DeploymentResponses
	limit := req.Pagination.Limit
	offset := req.Pagination.Offset
	skipped := uint64(0)
	countTotal := req.Pagination.CountTotal
	var total uint64
	var acctErr error

	for _, state := range states {
		if limit == 0 && !countTotal {
			break
		}

		var iter indexes.MultiIterator[int32, keys.DeploymentPrimaryKey]
		var err error
		if req.Pagination.Reverse {
			iter, err = k.deployments.Indexes.State.Iterate(ctx,
				collections.NewPrefixedPairRange[int32, keys.DeploymentPrimaryKey](int32(state)).Descending())
		} else {
			iter, err = k.deployments.Indexes.State.MatchExact(ctx, int32(state))
		}
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		err = indexes.ScanValues(ctx, k.deployments, iter, func(deployment v1.Deployment) bool {
			if !req.Filters.Accept(deployment, state) {
				return false
			}

			if countTotal {
				total++
			}

			if limit == 0 {
				return !countTotal
			}

			if skipped < offset {
				skipped++
				return false
			}

			account, acctE := k.ekeeper.GetAccount(ctx, deployment.ID.ToEscrowAccountID())
			if acctE != nil {
				acctErr = fmt.Errorf("%w: fetching escrow account for DeploymentID=%s", acctE, deployment.ID)
				return true
			}

			groups, grpErr := k.GetGroups(ctx, deployment.ID)
			if grpErr != nil {
				acctErr = fmt.Errorf("%w: fetching groups for DeploymentID=%s", grpErr, deployment.ID)
				return true
			}

			deployments = append(deployments, types.QueryDeploymentResponse{
				Deployment:    deployment,
				Groups:        groups,
				EscrowAccount: account,
			})
			limit--

			return false
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if acctErr != nil {
			return nil, status.Error(codes.Internal, acctErr.Error())
		}
	}

	resp := &types.QueryDeploymentsResponse{
		Deployments: deployments,
		Pagination:  &sdkquery.PageResponse{},
	}

	if countTotal {
		resp.Pagination.Total = total
	}

	return resp, nil
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
		return nil, v1.ErrDeploymentNotFound
	}

	account, err := k.ekeeper.GetAccount(ctx, req.ID.ToEscrowAccountID())
	if err != nil {
		return &types.QueryDeploymentResponse{}, err
	}

	groups, err := k.GetGroups(ctx, req.ID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryDeploymentResponse{
		Deployment:    deployment,
		Groups:        groups,
		EscrowAccount: account,
	}, nil
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
		return nil, v1.ErrGroupNotFound
	}

	return &types.QueryGroupResponse{Group: group}, nil
}

func (k Querier) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params := k.GetParams(sdkCtx)

	return &types.QueryParamsResponse{Params: params}, nil
}
