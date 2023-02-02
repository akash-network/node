package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"
)

func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
