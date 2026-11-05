package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/oracle/v2"
)

// InitGenesis initiate genesis state and return updated validator details
func (k *keeper) InitGenesis(ctx sdk.Context, data *types.GenesisState) {
	err := k.SetParams(ctx, data.Params)
	if err != nil {
		panic(err.Error())
	}

	if err := k.sourceSequence.Set(ctx, data.SourceSeq); err != nil {
		panic(err.Error())
	}

	for _, p := range data.Prices {
		if err := k.prices.Set(ctx, p.ID, p.State); err != nil {
			panic(err.Error())
		}
	}

	for _, p := range data.LatestPricesIDs {
		if err := k.latestPriceID.Set(ctx, *p.ID, *p.State); err != nil {
			panic(err.Error())
		}
	}

	for _, id := range data.SourceIDs {
		if err := k.sourceID.Set(ctx, id.Address, id.ID); err != nil {
			panic(err.Error())
		}
	}
}

// ExportGenesis returns genesis state for the deployment module
func (k *keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	params, err := k.GetParams(ctx)
	if err != nil {
		panic(err)
	}

	prices := make([]types.PriceData, 0)
	latestIDs := make([]types.GenesisLatestPricesIDs, 0)
	sourceIDs := make([]*types.GenesisSourceID, 0)

	err = k.prices.Walk(ctx, nil, func(key types.PriceDataRecordID, val types.PriceDataState) (stop bool, err error) {
		prices = append(prices, types.PriceData{
			ID:    key,
			State: val,
		})

		return false, nil
	})
	if err != nil {
		panic(err)
	}

	err = k.latestPriceID.Walk(ctx, nil, func(key types.PriceDataID, value types.PriceLatestDataState) (stop bool, err error) {
		latestIDs = append(latestIDs, types.GenesisLatestPricesIDs{
			ID: &types.PriceDataID{
				Source:    key.Source,
				Denom:     key.Denom,
				BaseDenom: key.BaseDenom,
			},
			State: &types.PriceLatestDataState{
				Timestamp: value.Timestamp,
				Sequence:  value.Sequence,
			},
		})

		return false, nil
	})
	if err != nil {
		panic(err)
	}

	err = k.sourceID.Walk(ctx, nil, func(key string, value uint32) (stop bool, err error) {
		sourceIDs = append(sourceIDs, &types.GenesisSourceID{
			Address: key,
			ID:      value,
		})
		return false, nil
	})

	seq, err := k.sourceSequence.Peek(ctx)
	if err != nil {
		panic(err.Error())
	}

	return &types.GenesisState{
		Params:          params,
		Prices:          prices,
		LatestPricesIDs: latestIDs,
		SourceIDs:       sourceIDs,
		SourceSeq:       seq,
	}
}
