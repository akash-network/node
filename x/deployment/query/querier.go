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
		case filterDepsPath:
			return queryFilterDeployments(ctx, path[1:], req, keeper)
		case deploymentPath:
			return queryDeployment(ctx, path[1:], req, keeper)
		case groupPath:
			return queryGroup(ctx, path[1:], req, keeper)
		}
		return []byte{}, sdkerrors.ErrUnknownRequest
	}
}

func queryDeployments(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	var values Deployments
	keeper.WithDeployments(ctx, func(deployment types.Deployment) bool {
		value := Deployment{
			Deployment: deployment,
			Groups:     keeper.GetGroups(ctx, deployment.ID()),
		}
		values = append(values, value)
		return false
	})

	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryFilterDeployments(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {
	filter, err := ParseDepFiltersPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(types.ErrInternal, err.Error())
	}

	var values Deployments

	keeper.WithDeployments(ctx, func(deployment types.Deployment) bool {
		if filter.Owner.Empty() {
			if deployment.State == filter.State {
				value := Deployment{
					Deployment: deployment,
					Groups:     keeper.GetGroups(ctx, deployment.ID()),
				}
				values = append(values, value)
			}
		} else if filter.State == 100 {
			if deployment.DeploymentID.Owner.Equals(filter.Owner) {
				value := Deployment{
					Deployment: deployment,
					Groups:     keeper.GetGroups(ctx, deployment.ID()),
				}
				values = append(values, value)
			}
		} else {
			if deployment.DeploymentID.Owner.Equals(filter.Owner) && deployment.State == filter.State {
				value := Deployment{
					Deployment: deployment,
					Groups:     keeper.GetGroups(ctx, deployment.ID()),
				}
				values = append(values, value)
			}
		}
		return false
	})

	return sdkutil.RenderQueryResponse(keeper.Codec(), values)
}

func queryDeployment(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

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

func queryGroup(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

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
