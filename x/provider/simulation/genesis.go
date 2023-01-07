package simulation

import (
	types "github.com/akash-network/node/x/provider/types/v1beta2"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	providerGenesis := &types.GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(providerGenesis)
}
