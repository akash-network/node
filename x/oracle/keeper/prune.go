package keeper

import (
	"context"
	"math"
	"time"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	epochstypes "pkg.akt.dev/go/node/epochs/v1beta1"
	types "pkg.akt.dev/go/node/oracle/v2"
)

var _ epochstypes.EpochHooks = &keeper{}

// AfterEpochEnd is called at the end of each epoch. If the epoch matches the
// configured prune_epoch, it prunes price records older than price_retention.
func (k *keeper) AfterEpochEnd(ctx context.Context, epochIdentifier string, _ int64) error {
	sctx := sdk.UnwrapSDKContext(ctx)

	params, err := k.GetParams(sctx)
	if err != nil {
		return err
	}

	if epochIdentifier != params.PruneEpoch {
		return nil
	}

	cutoff := sctx.BlockTime().Add(-params.PriceRetention)

	pruned, err := k.prunePrices(sctx, cutoff, params.MaxPrunePerEpoch)
	if err != nil {
		k.Logger(sctx).Error("failed to prune price records", "error", err)
	}

	if pruned > 0 {
		k.Logger(sctx).Info("pruned old price records",
			"pruned", pruned,
			"cutoff", cutoff.UTC().Format(time.RFC3339),
		)
	}

	return nil
}

// BeforeEpochStart is a no-op for the oracle module.
func (k *keeper) BeforeEpochStart(_ context.Context, _ string, _ int64) error {
	return nil
}

// prunePrices deletes price records older than cutoff using per-source prefix
// walks. Within each (source, denom, baseDenom) prefix, records are sorted by
// timestamp so the walk only touches prunable records. Returns total deleted.
func (k *keeper) prunePrices(ctx sdk.Context, cutoff time.Time, maxDelete int64) (int64, error) {
	var totalDeleted int64

	// Walk latestPriceID to get all (source, denom, baseDenom) combos
	err := k.latestPriceID.Walk(ctx, nil, func(key types.PriceDataID, _ types.PriceLatestDataState) (bool, error) {
		if totalDeleted >= maxDelete {
			return true, nil
		}

		deleted, err := k.pruneSourcePrices(ctx, key.Source, key.Denom, key.BaseDenom, cutoff, maxDelete-totalDeleted)
		totalDeleted += deleted

		return false, err
	})

	return totalDeleted, err
}

// pruneSourcePrices deletes price records for a single (source, denom, baseDenom)
// prefix that are older than cutoff. Returns number of records deleted.
func (k *keeper) pruneSourcePrices(ctx sdk.Context, source uint32, denom, baseDenom string, cutoff time.Time, maxDelete int64) int64 {
	start := types.PriceDataRecordID{
		Source:    source,
		Denom:     denom,
		BaseDenom: baseDenom,
	}

	end := types.PriceDataRecordID{
		Source:    source,
		Denom:     denom,
		BaseDenom: baseDenom,
		Timestamp: cutoff,
		Sequence:  math.MaxUint64,
	}

	rng := new(collections.Range[types.PriceDataRecordID]).
		StartInclusive(start).
		EndInclusive(end)

	var toDelete []types.PriceDataRecordID
	var count int64

	_ = k.prices.Walk(ctx, rng, func(key types.PriceDataRecordID, _ types.PriceDataState) (bool, error) {
		toDelete = append(toDelete, key)
		count++
		return count >= maxDelete, nil
	})

	for _, key := range toDelete {
		_ = k.prices.Remove(ctx, key)
	}

	return count
}

// EpochHooks returns the oracle keeper as an EpochHooks implementation for
// use with the x/epochs module.
func (k *keeper) EpochHooks() epochstypes.EpochHooks {
	return k
}

// EpochHooksFor returns an EpochHooks wrapper suitable for passing to
// epochs.SetHooks via MultiEpochHooks.
func EpochHooksFor(k Keeper) epochstypes.EpochHooks {
	return k.(*keeper)
}
