// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	"pkg.akt.dev/go/node/migrate"

	utypes "pkg.akt.dev/node/upgrades/types"
	ekeeper "pkg.akt.dev/node/x/escrow/keeper"
)

type escrowMigrations struct {
	utypes.Migrator
}

func newEscrowMigration(m utypes.Migrator) utypes.Migration {
	return escrowMigrations{Migrator: m}
}

func (m escrowMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates escrow store from version 2 to 3.
func (m escrowMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	if err := migrateAccounts(store, m.Codec()); err != nil {
		return err
	}

	if err := migratePayments(store, m.Codec()); err != nil {
		return err
	}

	return nil
}

func migrateAccounts(store storetypes.KVStore, cdc codec.BinaryCodec) (err error) {
	oStore := prefix.NewStore(store, migrate.AccountV1beta3Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.AccountFromV1beta3(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		key := ekeeper.AccountKey(nVal.ID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}

func migratePayments(store storetypes.KVStore, cdc codec.BinaryCodec) (err error) {
	oStore := prefix.NewStore(store, migrate.PaymentV1beta3Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.FractionalPaymentFromV1beta3(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		key := ekeeper.PaymentKey(nVal.AccountID, nVal.PaymentID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}
