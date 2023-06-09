package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
)

var (
	minDeposit, _ = types.DefaultParams().MinDepositFor("uakt")
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{
		Params: types.Params{
			MinDeposits: map[string]uint32{
				minDeposit.Denom: uint32(minDeposit.Amount.ToDec().TruncateInt64()),
			},
		},
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
