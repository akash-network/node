package simulation

import (
	types "github.com/akash-network/akash-api/go/node/take/v1beta3"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	takeGenesis := &types.GenesisState{
		Params: types.DefaultParams(),
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(takeGenesis)
}
