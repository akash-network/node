package keeper

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"

	types "pkg.akt.dev/go/node/oracle/v2"
)

// BeginBlocker checks if prices are being updated and sources do not deviate from each other
// price for requested denom halts if any of the following conditions occur
// - the price has not been updated within UpdatePeriod
// - price deviation between multiple sources is more than TBD
func (k *keeper) BeginBlocker(_ context.Context) error {
	return nil
}

// EndBlocker is called at the end of each block to aggregate oracle prices.
// It reads each source's latest price ID, then does a single price-history walk
// per denom to collect all data points within the TWAP window. Aggregation,
// TWAP, median, and health checks are computed from that in-memory data.
func (k *keeper) EndBlocker(ctx context.Context) error {
	start := telemetry.Now()
	defer telemetry.ModuleMeasureSince(types.ModuleName, start, telemetry.MetricKeyEndBlocker)

	sctx := sdk.UnwrapSDKContext(ctx)

	params, err := k.GetParams(sctx)
	if err != nil {
		sctx.Logger().Error("failed to get oracle params", "error", err)
		return nil
	}

	now := sctx.BlockTime()
	cutoffTime := now.Add(-params.MaxPriceStalenessPeriod)
	twapDuration := params.TwapWindow
	twapStart := now.Add(-twapDuration)

	// Build a set of currently-authorized source IDs from params.Sources.
	// Only these sources should participate in aggregation; latestPriceID
	// entries for removed sources are ignored.
	activeSourceIDs := make(map[uint32]bool, len(params.Sources))
	for _, source := range params.Sources {
		if id, err := k.sourceID.Get(sctx, source); err == nil {
			activeSourceIDs[id] = true
		}
	}

	// Phase 1: walk latestPriceID to discover sources per denom and their latest timestamps.
	// latestByDenom maps DataID → list of (source, latestTimestamp) pairs.
	type sourceInfo struct {
		source          uint32
		latestTimestamp time.Time
	}

	latestByDenom := make(map[types.DataID][]sourceInfo)

	err = k.latestPriceID.Walk(sctx, nil, func(key types.PriceDataID, state types.PriceLatestDataState) (bool, error) {
		// Skip sources that are no longer in params.Sources.
		if !activeSourceIDs[key.Source] {
			return false, nil
		}

		did := types.DataID{
			Denom:     key.Denom,
			BaseDenom: key.BaseDenom,
		}

		latestByDenom[did] = append(latestByDenom[did], sourceInfo{
			source:          key.Source,
			latestTimestamp: state.Timestamp,
		})

		return false, nil
	})
	if err != nil {
		panic(fmt.Sprintf("failed to walk latest prices: %v", err))
	}

	var evts []proto.Message

	// Sort DataID keys for deterministic iteration order.
	sortedDIDs := make([]types.DataID, 0, len(latestByDenom))
	for did := range latestByDenom {
		sortedDIDs = append(sortedDIDs, did)
	}
	sort.Slice(sortedDIDs, func(i, j int) bool {
		if sortedDIDs[i].Denom != sortedDIDs[j].Denom {
			return sortedDIDs[i].Denom < sortedDIDs[j].Denom
		}
		return sortedDIDs[i].BaseDenom < sortedDIDs[j].BaseDenom
	})

	for _, did := range sortedDIDs {
		sources := latestByDenom[did]
		// Phase 2: for each source, do a single walk over [twapStart, now] to get all
		// data points. This serves both the staleness filter and TWAP computation.
		// sourcePrices[sourceID] → sorted price data points (descending by timestamp from walk)
		sourcePrices := make(map[uint32][]types.PriceData, len(sources))
		var latestPrices []types.PriceData
		allSourceIDs := make([]types.PriceDataRecordID, 0, len(sources))

		for _, si := range sources {
			rID := types.PriceDataRecordID{
				Source:    si.source,
				Denom:     did.Denom,
				BaseDenom: did.BaseDenom,
				Timestamp: si.latestTimestamp,
			}
			allSourceIDs = append(allSourceIDs, rID)

			// single range walk for this source over the TWAP window
			history := k.getTWAPHistory(sctx, si.source, did.Denom, did.BaseDenom, twapStart, now)
			if len(history) == 0 {
				continue
			}

			sourcePrices[si.source] = history

			// latest price is first element (descending order)
			latest := history[0]
			if !latest.ID.Timestamp.Before(cutoffTime) {
				latestPrices = append(latestPrices, latest)
			}
		}

		// Phase 3: aggregate from in-memory data
		aggregatedPrice, err := k.calculateAggregatedPricesFromHistory(sctx, did, latestPrices, sourcePrices)
		if err != nil {
			sctx.Logger().Error(
				"calculate aggregated price",
				"reason", err.Error(),
			)
		}

		health := k.setPriceHealth(sctx, params, allSourceIDs, aggregatedPrice)

		if health.IsHealthy && len(latestPrices) > 0 {
			err = k.aggregatedPrices.Set(sctx, did, aggregatedPrice)
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
