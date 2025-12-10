package simulation

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	mv1 "pkg.akt.dev/go/node/market/v2beta1"

	dtypes "pkg.akt.dev/go/node/deployment/v1beta5"
	types "pkg.akt.dev/go/node/market/v2beta1"
)

var minDeposit, _ = dtypes.DefaultParams().MinDepositFor("uakt")

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	marketGenesis := &types.GenesisState{
		Params: types.Params{
			BidMinDeposit: minDeposit,
			OrderMaxBids:  20,
		},
	}

	simState.GenState[mv1.ModuleName] = simState.Cdc.MustMarshalJSON(marketGenesis)
}
