package keeper

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"

	types "pkg.akt.dev/go/node/oracle/v1"
)

// BeginBlocker checks if prices are being updated and sources do not deviate from each other
// price for requested denom halts if any of the following conditions occur
// - the price has not been updated within UpdatePeriod
// - price deviation between multiple sources is more than TBD
func (k *keeper) BeginBlocker(_ context.Context) error {
	return nil
}

// EndBlocker is called at the end of each block to manage snapshots.
// It records periodic snapshots and prunes old ones.
func (k *keeper) EndBlocker(ctx context.Context) error {
	start := telemetry.Now()
	defer telemetry.ModuleMeasureSince(types.ModuleName, start, telemetry.MetricKeyBeginBlocker)

	sctx := sdk.UnwrapSDKContext(ctx)

	params, _ := k.GetParams(sctx)

	rIDs := make(map[types.DataID][]types.PriceDataRecordID)

	err := k.latestPrices.Walk(sctx, nil, func(key types.PriceDataID, height int64) (bool, error) {
		dataID := types.DataID{
			Denom:     key.Denom,
			BaseDenom: key.BaseDenom,
		}

		rID := types.PriceDataRecordID{
			Source:    key.Source,
			Denom:     key.Denom,
			BaseDenom: key.BaseDenom,
			Height:    height,
		}

		data, exists := rIDs[dataID]
		if !exists {
			data = []types.PriceDataRecordID{rID}
		} else {
			data = append(data, rID)
		}

		rIDs[dataID] = data

		return false, nil
	})

	if err != nil {
		panic(fmt.Sprintf("failed to walk latest prices: %v", err))
	}

	cutoffHeight := sctx.BlockHeight() - params.MaxPriceStalenessBlocks

	var evts []proto.Message

	for id, rid := range rIDs {
		latestData := make([]types.PriceData, 0, len(rid))

		for _, id := range rid {
			if id.Height < cutoffHeight {
				continue
			}

			state, _ := k.prices.Get(sctx, id)

			latestData = append(latestData, types.PriceData{
				ID:    id,
				State: state,
			})
		}

		// Aggregate prices from all active sources
		aggregatedPrice, err := k.calculateAggregatedPrices(sctx, id, latestData)
		if err != nil {
			sctx.Logger().Error(
				"calculate aggregated price",
				"reason", err.Error(),
			)
		}

		health := k.setPriceHealth(sctx, params, rid, aggregatedPrice)

		// If healthy and we have price data, update the final oracle price
		if health.IsHealthy && len(latestData) > 0 {
			err = k.aggregatedPrices.Set(sctx, id, aggregatedPrice)
			if err != nil {
				sctx.Logger().Error(
					"set aggregated price",
					"reason", err.Error(),
				)
			}

			evts = append(evts, &types.EventAggregatedPrice{Price: aggregatedPrice})
		}
	}

	err = sctx.EventManager().EmitTypedEvents(evts...)
	if err != nil {
		sctx.Logger().Error("failed to emit oracle price status change event", "error", err)
	}

	return nil
}
