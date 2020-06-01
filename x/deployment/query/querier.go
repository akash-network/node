package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// NewQuerier creates and returns a new deployment querier instance
func NewQuerier(keeper keeper.Keeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
		switch path[0] {
		case deploymentsPath:
			return queryDeployments(ctx, path[1:], req, keeper)
		case deploymentPath:
			return queryDeployment(ctx, path[1:], req, keeper)
		case groupPath:
			return queryGroup(ctx, path[1:], req, keeper)
		}
		return []byte{}, sdkerrors.ErrUnknownRequest
	}
}

func queryDeployments(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	// isValidState denotes whether given state flag is valid or not
	filters, isValidState, err := parseDepFiltersPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	var values Deployments
	keeper.WithDeployments(ctx, func(deployment types.Deployment) bool {
		if (filters.Owner.Empty() && !isValidState) ||
			(filters.Owner.Empty() && (deployment.State == filters.State)) ||
			(!isValidState && (deployment.DeploymentID.Owner.Equals(filters.Owner))) ||
			(deployment.DeploymentID.Owner.Equals(filters.Owner) && deployment.State == filters.State) {
			value := Deployment{
				Deployment: deployment,
				Groups:     keeper.GetGroups(ctx, deployment.ID()),
			}
			values = append(values, value)
		}

		return false
	})

	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryDeployment(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

	id, err := parseDeploymentPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	deployment, ok := keeper.GetDeployment(ctx, id)
	if !ok {
		return nil, types.ErrDeploymentNotFound
	}

	value := Deployment{
		Deployment: deployment,
		Groups:     keeper.GetGroups(ctx, deployment.ID()),
	}

	return sdkutil.RenderQueryResponse(keeper.Codec(), value)
}

func queryGroup(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

	id, err := ParseGroupPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(err, "internal error")
	}

	group, ok := keeper.GetGroup(ctx, id)
	if !ok {
		return nil, sdkerrors.Wrap(err, "group not found")
	}

	value := Group(group)

	return sdkutil.RenderQueryResponse(keeper.Codec(), value)
}
