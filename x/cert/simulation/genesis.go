package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"

	types "pkg.akt.dev/go/node/cert/v1"
)

func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
