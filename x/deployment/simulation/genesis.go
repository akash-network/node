package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/ovrclk/akash/x/deployment/types"
)

// GenesisDeployment defines the basic genesis state used by deployment module
type GenesisDeployment struct {
	types.Deployment
	Groups []types.Group
}

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
