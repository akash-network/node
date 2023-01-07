package simulation

import (
	types "github.com/akash-network/node/x/market/types/v1beta2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

var minDeposit = sdk.NewInt64Coin("stake", 1)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	marketGenesis := &types.GenesisState{
		Params: types.Params{
			BidMinDeposit: minDeposit,
			OrderMaxBids:  20,
		},
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(marketGenesis)
}
