// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	utypes "pkg.akt.dev/node/upgrades/types"
	mkeys "pkg.akt.dev/node/x/market/keeper/keys"

	"pkg.akt.dev/go/node/migrate"
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

// handler migrates market from version 5 to 6.
// TODO @troian see if prefixes need to be updated
func (m marketMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	if err := migrateOrders(store, m.Codec()); err != nil {
		return err
	}

	if err := migrateBids(store, m.Codec()); err != nil {
		return err
	}

	if err := migrateLeases(store, m.Codec()); err != nil {
		return err
	}

	return nil
}

func migrateOrders(store sdk.KVStore, cdc codec.BinaryCodec) (err error) {
	oStore := prefix.NewStore(store, migrate.OrderV1beta4Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.OrderFromV1beta4(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		key := mkeys.OrderKey(nVal.ID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}

func migrateBids(store sdk.KVStore, cdc codec.BinaryCodec) (err error) {
	oStore := prefix.NewStore(store, migrate.BidV1beta4Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.BidFromV1beta4(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		key := mkeys.BidKey(nVal.ID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}

func migrateLeases(store sdk.KVStore, cdc codec.BinaryCodec) (err error) {
	oStore := prefix.NewStore(store, migrate.LeaseV1beta4Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.LeaseFromV1beta4(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		key := mkeys.LeaseKey(nVal.ID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}
