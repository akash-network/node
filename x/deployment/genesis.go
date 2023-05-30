package deployment

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	abci "github.com/tendermint/tendermint/abci/types"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	"github.com/akash-network/node/x/deployment/keeper"
)

// ValidateGenesis does validation check of the Genesis and return error in case of failure
func ValidateGenesis(data *types.GenesisState) error {
	for _, record := range data.Deployments {
		if err := record.Deployment.ID().Validate(); err != nil {
			return fmt.Errorf("%w: %s", err, types.ErrInvalidDeployment.Error())
		}
	}
	return data.Params.Validate()
}

// DefaultGenesisState returns default genesis state as raw bytes for the deployment
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{
		Params: types.DefaultParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.IKeeper, data *types.GenesisState) []abci.ValidatorUpdate {
	for _, record := range data.Deployments {
		if err := keeper.Create(ctx, record.Deployment, record.Groups); err != nil {
			return nil
		}
	}
	keeper.SetParams(ctx, data.Params)
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	var records []types.GenesisDeployment
	k.WithDeployments(ctx, func(deployment types.Deployment) bool {
		groups := k.GetGroups(ctx, deployment.ID())
		records = append(records, types.GenesisDeployment{
			Deployment: deployment,
			Groups:     groups,
		})
		return false
	})

	params := k.GetParams(ctx)
	return &types.GenesisState{
		Deployments: records,
		Params:      params,
	}
}

// GetGenesisStateFromAppState returns x/deployment GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *types.GenesisState {
	var genesisState types.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
