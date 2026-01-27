package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/oracle/v1"
)

// InitGenesis initiate genesis state and return updated validator details
func (k *keeper) InitGenesis(ctx sdk.Context, data *types.GenesisState) {
	err := k.SetParams(ctx, data.Params)
	if err != nil {
		panic(err.Error())
	}

	//for _, p := range data.Prices {
	//
	//}
	//
	//for _, h := range data.LatestHeight {
	//
	//}
}

// ExportGenesis returns genesis state for the deployment module
func (k *keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	params, err := k.GetParams(ctx)
	if err != nil {
		panic(err)
	}

	//prices := make([]types.PriceEntry, 0)
	//latestHeights := make([]types.PriceEntryID, 0)
	//
	//k.WithPriceEntries(ctx, func(val types.PriceEntry) bool {
	//	prices = append(prices, val)
	//	return false
	//})
	//
	//k.WithLatestHeights(ctx, func(val types.PriceEntryID) bool {
	//	latestHeights = append(latestHeights, val)
	//	return false
	//})

	return &types.GenesisState{
		Params: params,
		//Prices:       prices,
		//LatestHeight: latestHeights,
	}
}
