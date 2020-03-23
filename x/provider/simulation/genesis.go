package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/ovrclk/akash/x/provider/types"
)

// GenesisState defines the basic genesis state used by provider module
type GenesisState struct {
	Providers []types.Provider `json:"providers"`
}

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	providerGenesis := GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(providerGenesis)
}
