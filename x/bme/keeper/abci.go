package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/bme/v1"
	otypes "pkg.akt.dev/go/node/oracle/v1"
	"pkg.akt.dev/go/sdkutil"
)

// BeginBlocker is called at the beginning of each block
func (k *keeper) BeginBlocker(_ context.Context) error {
	// reset the ledger sequence on each new block
	// sequence must start from 1 for ledger record id range to work correctly
	k.ledgerSequence = 1

	return nil
}

// EndBlocker is called at the end of each block to manage snapshots.
// It records periodic snapshots and prunes old ones.
func (k *keeper) EndBlocker(ctx context.Context) error {
	startTm := telemetry.Now()
	defer telemetry.ModuleMeasureSince(types.ModuleName, startTm, telemetry.MetricKeyEndBlocker)

	sctx := sdk.UnwrapSDKContext(ctx)

	params, err := k.GetParams(sctx)
	if err != nil {
		panic(err)
	}

	var processed int64

	executeMint := func(id types.LedgerRecordID, value types.LedgerPendingRecord) (bool, error) {
		ownerAddr, err := k.ac.StringToBytes(value.Owner)
		if err != nil {
			return false, err
		}

		dstAddr, err := k.ac.StringToBytes(value.To)
		if err != nil {
			return false, err
		}

		// Use CacheContext so that a partial failure (e.g. MintCoins succeeds
		// but SendCoinsFromModuleToAccount fails) does not leave a corrupted
		// state. Only commit on success; on error the pending record stays
		// unmodified and will be retried next block.
		cacheCtx, writeCache := sctx.CacheContext()
		err = k.executeBurnMint(cacheCtx, params, id, ownerAddr, dstAddr, value.CoinsToBurn, value.DenomToMint)
		if err == nil {
			writeCache()
		}

		processed++
		return processed >= int64(params.MaxEndblockerRecords), err
	}

	iteratePending := func(p []byte, postCondition func() error) error {
		ss := prefix.NewStore(sctx.KVStore(k.skey), k.ledgerPending.GetPrefix())

		iter := storetypes.KVStorePrefixIterator(ss, p)
		defer func() {
			if err := iter.Close(); err != nil {
				sctx.Logger().Error("closing ledger pending iterator", "err", err)
			}
		}()

		stop := false

		for ; !stop && iter.Valid(); iter.Next() {
			_, id, err := ledgerRecordIDCodec{}.Decode(iter.Key())
			if err != nil {
				panic(err)
			}

			var val types.LedgerPendingRecord
			k.cdc.MustUnmarshal(iter.Value(), &val)

			stop, err = executeMint(id, val)
			if err != nil {
				sctx.Logger().Error("processing ledger pending records ", "id", id, "err", err)
				if errors.Is(err, otypes.ErrPriceStalled) {
					return err
				}
			}
		}

		return postCondition()
	}

	be, err := k.epochs.Get(sctx, epochBurn)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		panic(err)
	}

	nextBEpoch := be

	if be <= sctx.BlockHeight() {
		be = sctx.BlockHeight() + params.MinEpochBlocks

		pid := types.LedgerRecordID{
			Denom:   sdkutil.DenomUact,
			ToDenom: sdkutil.DenomUakt,
		}

		startPrefix, err := ledgerRecordIDCodec{}.ToPrefix(pid)
		if err != nil {
			panic(err)
		}

		err = iteratePending(startPrefix, func() error {
			return nil
		})
		if err != nil {
			sctx.Logger().Error("walking ledger pending records", "prefix", pid, "err", err)
		}
	}

	if nextBEpoch != be {
		if err = k.epochs.Set(sctx, epochBurn, be); err != nil {
			panic(err)
		}
	}

	cr, crUpdated := k.mintStatusUpdate(sctx)

	me, err := k.epochs.Get(sctx, epochMint)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		panic(err)
	}

	nextMEpoch := me

	// if circuit breaker was just reset then calculate next epoch
	if crUpdated && (cr.PreviousStatus >= types.MintStatusHaltCR) && (cr.Status <= types.MintStatusWarning) {
		me = sctx.BlockHeight() + cr.EpochHeightDiff
	} else if (cr.Status <= types.MintStatusWarning) && (me == sctx.BlockHeight()) {
		me = sctx.BlockHeight() + cr.EpochHeightDiff

		pid := types.LedgerRecordID{
			Denom:   sdkutil.DenomUakt,
			ToDenom: sdkutil.DenomUact,
		}

		startPrefix, err := ledgerRecordIDCodec{}.ToPrefix(pid)
		if err != nil {
			panic(err)
		}

		err = iteratePending(startPrefix, func() error {
			cr, _ := k.mintStatusUpdate(sctx)
			if cr.Status >= types.MintStatusHaltCR {
				return types.ErrCircuitBreakerActive
			}
			return nil
		})
		if err != nil {
			sctx.Logger().Error("walking ledger records", "prefix", pid, "err", err)
		}
	}

	if nextMEpoch != me {
		if err = k.epochs.Set(sctx, epochMint, me); err != nil {
			panic(err)
		}
	}

	return nil
}
