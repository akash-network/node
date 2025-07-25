package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"

	types "pkg.akt.dev/go/node/provider/v1beta4"
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	providerGenesis := &types.GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(providerGenesis)
}
