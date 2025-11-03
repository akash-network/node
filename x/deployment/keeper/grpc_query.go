package keeper

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/indexes"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	v1 "pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/v2/util/query"
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
	} else if req.Pagination.Offset > 0 && req.Filters.State == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.state must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	ctx := sdk.UnwrapSDKContext(c)

	// Step 1: Resolve states, resumePK, and iteration path
	states := make([]byte, 0, 2)
	var resumePK *keys.DeploymentPrimaryKey
	var ownerPath bool
	var owner string

	if len(req.Pagination.Key) > 0 {
		// RESUME — all filters ignored, key provides everything
		var pkBytes, unsolicited []byte
		var err error
		states, _, pkBytes, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		_, pk, err := k.deployments.KeyCodec().Decode(pkBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resumePK = &pk

		if len(unsolicited) > 0 && unsolicited[0] == 1 {
			ownerPath = true
			owner = resumePK.K1()
		}
	} else {
		// INITIAL — resolve states from Filters.State
		if req.Filters.State != "" {
			stateVal := v1.Deployment_State(v1.Deployment_State_value[req.Filters.State])
			if stateVal == v1.DeploymentStateInvalid {
				return nil, status.Error(codes.InvalidArgument, "invalid state value")
			}
			states = append(states, byte(stateVal))
		} else {
			states = append(states, byte(v1.DeploymentActive), byte(v1.DeploymentClosed))
		}

		// Resolve iteration path from Filters.Owner
		if req.Filters.Owner != "" {
			ownerPath = true
			owner = req.Filters.Owner
		}
	}

	// Step 2: Direct Get path — Owner + DSeq = full PK known
	if ownerPath && resumePK == nil && req.Filters.DSeq != 0 {
		return k.deploymentsDirectGet(ctx, req, states)
	}

	// Step 3: Owner path — iterate primary map with owner prefix
	if ownerPath {
		return k.deploymentsOwnerPath(ctx, req, states, owner, resumePK)
	}

	// Step 4: State-index path — iterate by state index
	if len(req.Pagination.Key) == 0 && req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	return k.deploymentsStatePath(ctx, req, states, resumePK)
}

// deploymentsDirectGet handles the case where Owner + DSeq are both set, giving a full primary key.
func (k Querier) deploymentsDirectGet(
	ctx sdk.Context,
	req *types.QueryDeploymentsRequest,
	states []byte,
) (*types.QueryDeploymentsResponse, error) {
	pk := collections.Join(req.Filters.Owner, req.Filters.DSeq)
	deployment, err := k.deployments.Get(ctx, pk)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return &types.QueryDeploymentsResponse{
				Pagination: &sdkquery.PageResponse{},
			}, nil
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check state filter
	stateMatch := false
	for _, s := range states {
		if v1.Deployment_State(s) == deployment.State {
			stateMatch = true
			break
		}
	}
	if !stateMatch {
		return &types.QueryDeploymentsResponse{
			Pagination: &sdkquery.PageResponse{},
		}, nil
	}

	account, err := k.ekeeper.GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("fetching escrow account for DeploymentID=%s: %v", deployment.ID, err))
	}

	groups, err := k.GetGroups(ctx, deployment.ID)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("fetching groups for DeploymentID=%s: %v", deployment.ID, err))
	}

	return &types.QueryDeploymentsResponse{
		Deployments: types.DeploymentResponses{
			{
				Deployment:    deployment,
				Groups:        groups,
				EscrowAccount: account,
			},
		},
		Pagination: &sdkquery.PageResponse{
			Total: 1,
		},
	}, nil
}

// deploymentsOwnerPath iterates the primary map with an owner prefix.
// States are filtered in the Walk callback.
func (k Querier) deploymentsOwnerPath(
	ctx sdk.Context,
	req *types.QueryDeploymentsRequest,
	states []byte,
	owner string,
	resumePK *keys.DeploymentPrimaryKey,
) (*types.QueryDeploymentsResponse, error) {
	// Build state set for callback filtering
	stateSet := make(map[v1.Deployment_State]bool, len(states))
	for _, s := range states {
		stateSet[v1.Deployment_State(s)] = true
	}

	// Build range on primary map
	ownerRange := collections.NewPrefixedPairRange[string, uint64](owner)

	var r *collections.PairRange[string, uint64]
	if resumePK != nil {
		if req.Pagination.Reverse {
			r = collections.NewPrefixedPairRange[string, uint64](owner).EndInclusive(resumePK.K2()).Descending()
		} else {
			r = collections.NewPrefixedPairRange[string, uint64](owner).StartInclusive(resumePK.K2())
		}
	} else if req.Pagination.Reverse {
		r = ownerRange.Descending()
	} else {
		r = ownerRange
	}

	var deployments types.DeploymentResponses
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset

	walkErr := k.deployments.Walk(ctx, r, func(_ keys.DeploymentPrimaryKey, deployment v1.Deployment) (bool, error) {
		// State filter
		if !stateSet[deployment.State] {
			return false, nil
		}

		// Offset
		if offset > 0 {
			offset--
			return false, nil
		}

		// Page full — encode NextKey
		if req.Pagination.Limit == 0 {
			npk := keys.DeploymentIDToKey(deployment.ID)
			pkBuf := make([]byte, k.deployments.KeyCodec().Size(npk))
			if _, err := k.deployments.KeyCodec().Encode(pkBuf, npk); err != nil {
				return true, err
			}
			var err error
			nextKey, err = query.EncodePaginationKey(states, []byte{states[0]}, pkBuf, []byte{1})
			if err != nil {
				return true, err
			}
			return true, nil
		}

		// Collect result
		account, err := k.ekeeper.GetAccount(ctx, deployment.ID.ToEscrowAccountID())
		if err != nil {
			return true, fmt.Errorf("%w: fetching escrow account for DeploymentID=%s", err, deployment.ID)
		}

		groups, err := k.GetGroups(ctx, deployment.ID)
		if err != nil {
			return true, fmt.Errorf("%w: fetching groups for DeploymentID=%s", err, deployment.ID)
		}

		deployments = append(deployments, types.QueryDeploymentResponse{
			Deployment:    deployment,
			Groups:        groups,
			EscrowAccount: account,
		})
		req.Pagination.Limit--
		total++
		return false, nil
	})
	if walkErr != nil {
		return nil, status.Error(codes.Internal, walkErr.Error())
	}

	return &types.QueryDeploymentsResponse{
		Deployments: deployments,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

// deploymentsStatePath iterates deployments via the State index.
func (k Querier) deploymentsStatePath(
	ctx sdk.Context,
	req *types.QueryDeploymentsRequest,
	states []byte,
	resumePK *keys.DeploymentPrimaryKey,
) (*types.QueryDeploymentsResponse, error) {
	var deployments types.DeploymentResponses
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

	for idx := range states {
		if req.Pagination.Limit == 0 && len(nextKey) > 0 {
			break
		}

		state := v1.Deployment_State(states[idx])

		var iter indexes.MultiIterator[int32, keys.DeploymentPrimaryKey]
		var err error

		if idx == 0 && resumePK != nil {
			r := collections.NewPrefixedPairRange[int32, keys.DeploymentPrimaryKey](int32(state)).StartInclusive(*resumePK)
			if req.Pagination.Reverse {
				r = collections.NewPrefixedPairRange[int32, keys.DeploymentPrimaryKey](int32(state)).EndInclusive(*resumePK).Descending()
			}
			iter, err = k.deployments.Indexes.State.Iterate(ctx, r)
		} else if req.Pagination.Reverse {
			iter, err = k.deployments.Indexes.State.Iterate(ctx,
				collections.NewPrefixedPairRange[int32, keys.DeploymentPrimaryKey](int32(state)).Descending())
		} else {
			iter, err = k.deployments.Indexes.State.MatchExact(ctx, int32(state))
		}
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		count := uint64(0)

		err = indexes.ScanValues(ctx, k.deployments, iter, func(deployment v1.Deployment) bool {
			if !req.Filters.Accept(deployment, state) {
				return false
			}

			if offset > 0 {
				offset--
				return false
			}

			if req.Pagination.Limit == 0 {
				// Page is full — encode this item's PK as NextKey
				pk := keys.DeploymentIDToKey(deployment.ID)
				pkBuf := make([]byte, k.deployments.KeyCodec().Size(pk))
				if _, encErr := k.deployments.KeyCodec().Encode(pkBuf, pk); encErr != nil {
					scanErr = encErr
					return true
				}
				var encErr error
				nextKey, encErr = query.EncodePaginationKey(states[idx:], []byte{states[idx]}, pkBuf, nil)
				if encErr != nil {
					scanErr = encErr
				}
				return true
			}

			account, acctErr := k.ekeeper.GetAccount(ctx, deployment.ID.ToEscrowAccountID())
			if acctErr != nil {
				scanErr = fmt.Errorf("%w: fetching escrow account for DeploymentID=%s", acctErr, deployment.ID)
				return true
			}

			groups, grpErr := k.GetGroups(ctx, deployment.ID)
			if grpErr != nil {
				scanErr = fmt.Errorf("%w: fetching groups for DeploymentID=%s", grpErr, deployment.ID)
				return true
			}

			deployments = append(deployments, types.QueryDeploymentResponse{
				Deployment:    deployment,
				Groups:        groups,
				EscrowAccount: account,
			})
			req.Pagination.Limit--
			count++

			return false
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if scanErr != nil {
			return nil, status.Error(codes.Internal, scanErr.Error())
		}

		total += count
	}

	return &types.QueryDeploymentsResponse{
		Deployments: deployments,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
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
