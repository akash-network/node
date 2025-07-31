package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"

	types "pkg.akt.dev/go/node/take/v1"
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	takeGenesis := &types.GenesisState{
		Params: types.DefaultParams(),
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(takeGenesis)
}
