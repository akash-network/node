package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

func NewQuerier(keeper keeper.Keeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, sdk.Error) {
		switch path[0] {
		case deploymentsPath:
			return queryDeployments(ctx, path[1:], req, keeper)
		case deploymentPath:
			return queryDeployment(ctx, path[1:], req, keeper)
		}
		return []byte{}, sdk.ErrUnknownRequest("unknown query path")
	}
}

func queryDeployments(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, sdk.Error) {

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

func queryDeployment(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, sdk.Error) {

	id, err := ParseDeploymentPath(path)
	if err != nil {
		return nil, sdk.ErrInternal(err.Error())
	}

	deployment, ok := keeper.GetDeployment(ctx, id)
	if !ok {
		return nil, sdk.ErrInternal("deployment not found")
	}

	value := Deployment{
		Deployment: deployment,
		Groups:     keeper.GetGroups(ctx, deployment.ID()),
	}

	return sdkutil.RenderQueryResponse(keeper.Codec(), value)
}
