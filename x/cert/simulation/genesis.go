package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/ovrclk/akash/x/cert/types"
)

func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
