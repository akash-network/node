package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

func NewQuerier(keeper keeper.Keeper) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
		switch path[0] {
		case deploymentsPath:
			return queryDeployments(ctx, path[1:], req, keeper)
		case deploymentPath:
			return queryDeployment(ctx, path[1:], req, keeper)
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

func queryDeployment(ctx sdk.Context, path []string, req abci.RequestQuery, keeper keeper.Keeper) ([]byte, error) {

	id, err := ParseDeploymentPath(path)
	if err != nil {
		return nil, sdkerrors.Wrap(err, "internal error")
	}

	deployment, ok := keeper.GetDeployment(ctx, id)
	if !ok {
		return nil, sdkerrors.Wrap(err, "deployment not found")
	}

	value := Deployment{
		Deployment: deployment,
		Groups:     keeper.GetGroups(ctx, deployment.ID()),
	}

	return sdkutil.RenderQueryResponse(keeper.Codec(), value)
}
