package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/ovrclk/akash/x/market/types"
)

// GenesisState stores slice of genesis market instance
type GenesisState struct {
	Orders []types.Order `json:"orders"`
	Leases []types.Lease `json:"leases"`
}

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	marketGenesis := GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(marketGenesis)
}
