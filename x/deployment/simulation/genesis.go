package simulation

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	types "github.com/ovrclk/akash/x/deployment/types/v1beta2"
)

var minDeposit = sdk.NewInt64Coin("stake", 1)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{
		Params: types.Params{
			DeploymentMinDeposit: minDeposit,
		},
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
