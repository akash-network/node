package simulation

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	types "github.com/ovrclk/akash/x/inflation/types/v1beta2"
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	deploymentGenesis := &types.GenesisState{
		Params: types.Params{
			InflationDecayFactor: 2,
			InitialInflation:     sdk.NewDec(100),
			Variance:             sdk.MustNewDecFromStr("0.05"),
		},
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
