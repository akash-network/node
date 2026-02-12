// Package v1_2_0
// nolint revive
package v1_2_0

import (
	"fmt"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"

	utypes "pkg.akt.dev/node/upgrades/types"
	mkeeper "pkg.akt.dev/node/x/market/keeper"
	mkeys "pkg.akt.dev/node/x/market/keeper/keys"
)

type marketMigrations struct {
	utypes.Migrator
}

func newMarketMigration(m utypes.Migrator) utypes.Migration {
	return marketMigrations{Migrator: m}
}

func (m marketMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates market from version 7 to 8.
// Moves orders, bids, and leases from manual KVStore keys to collections.IndexedMap.
func (m marketMigrations) handler(ctx sdk.Context) error {
	skey := m.StoreKey()
	cdc := m.Codec()
	store := ctx.KVStore(skey)

	// Build IndexedMaps locally (same construction as NewKeeper)
	ssvc := runtime.NewKVStoreService(skey.(*storetypes.KVStoreKey))
	sb := collections.NewSchemaBuilder(ssvc)

	orderIndexes := mkeeper.NewOrderIndexes(sb)
	orders := collections.NewIndexedMap(sb, collections.NewPrefix(mkeys.OrderPrefixNew), "orders", mkeys.OrderPrimaryKeyCodec, codec.CollValue[mtypes.Order](cdc), orderIndexes)

	bidIndexes := mkeeper.NewBidIndexes(sb)
	bids := collections.NewIndexedMap(sb, collections.NewPrefix(mkeys.BidPrefixNew), "bids", mkeys.BidPrimaryKeyCodec, codec.CollValue[mtypes.Bid](cdc), bidIndexes)

	leaseIndexes := mkeeper.NewLeaseIndexes(sb)
	leases := collections.NewIndexedMap(sb, collections.NewPrefix(mkeys.LeasePrefixNew), "leases", mkeys.LeasePrimaryKeyCodec, codec.CollValue[mv1.Lease](cdc), leaseIndexes)

	if _, err := sb.Build(); err != nil {
		return err
	}

	// === Orders ===
	oiter := storetypes.KVStorePrefixIterator(store, mkeys.OrderPrefix)
	defer func() {
		_ = oiter.Close()
	}()

	var orderCount int64
	var bidCount int64
	var leaseCount int64

	for ; oiter.Valid(); oiter.Next() {
		var order mtypes.Order
		cdc.MustUnmarshal(oiter.Value(), &order)

		pk := mkeys.OrderIDToKey(order.ID)
		if err := orders.Set(ctx, pk, order); err != nil {
			return fmt.Errorf("failed to migrate order %s: %w", order.ID, err)
		}

		store.Delete(oiter.Key())

		orderCount++
	}

	// === Bids ===
	biter := storetypes.KVStorePrefixIterator(store, mkeys.BidPrefix)
	defer func() {
		_ = biter.Close()
	}()

	for ; biter.Valid(); biter.Next() {
		var bid mtypes.Bid
		cdc.MustUnmarshal(biter.Value(), &bid)

		pk := mkeys.BidIDToKey(bid.ID)
		if err := bids.Set(ctx, pk, bid); err != nil {
			return fmt.Errorf("failed to migrate bid %s: %w", bid.ID, err)
		}

		store.Delete(biter.Key())
		bidCount++
	}

	// Delete old bid reverse keys
	brevIter := storetypes.KVStorePrefixIterator(store, mkeys.BidPrefixReverse)
	defer func() {
		_ = brevIter.Close()
	}()

	for ; brevIter.Valid(); brevIter.Next() {
		store.Delete(brevIter.Key())
	}

	// === Leases ===
	liter := storetypes.KVStorePrefixIterator(store, mkeys.LeasePrefix)
	defer func() {
		_ = liter.Close()
	}()

	for ; liter.Valid(); liter.Next() {
		var lease mv1.Lease
		cdc.MustUnmarshal(liter.Value(), &lease)

		pk := mkeys.LeaseIDToKey(lease.ID)
		if err := leases.Set(ctx, pk, lease); err != nil {
			return fmt.Errorf("failed to migrate lease %s: %w", lease.ID, err)
		}

		store.Delete(liter.Key())
		leaseCount++
	}

	// Delete old lease reverse keys
	lrevIter := storetypes.KVStorePrefixIterator(store, mkeys.LeasePrefixReverse)
	defer func() {
		_ = lrevIter.Close()
	}()

	for ; lrevIter.Valid(); lrevIter.Next() {
		store.Delete(lrevIter.Key())
	}

	ctx.Logger().Info("market store migration complete",
		"orders_migrated", orderCount,
		"bids_migrated", bidCount,
		"leases_migrated", leaseCount,
	)

	return nil
}
