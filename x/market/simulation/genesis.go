package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/ovrclk/akash/x/market/types"
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	marketGenesis := &types.GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(marketGenesis)
}
