package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"

	types "github.com/ovrclk/akash/x/cert/types/v1beta2"
)

func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
