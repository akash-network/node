package simulation

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	types "github.com/akash-network/akash-api/go/node/inflation/v1beta3"
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{
		Params: types.Params{
			InflationDecayFactor: types.DefaultInflationDecayFactor(),
			InitialInflation:     sdk.NewDec(100),
			Variance:             sdk.MustNewDecFromStr("0.05"),
		},
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
