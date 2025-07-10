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
	dkeeper "pkg.akt.dev/node/x/deployment/keeper"
)

type deploymentsMigrations struct {
	utypes.Migrator
}

func newDeploymentsMigration(m utypes.Migrator) utypes.Migration {
	return deploymentsMigrations{Migrator: m}
}

func (m deploymentsMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates deployment store from version 4 to 5
func (m deploymentsMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	if err := migrateDeployments(store, m.Codec()); err != nil {
		return err
	}

	if err := migrateGroups(store, m.Codec()); err != nil {
		return err
	}

	return nil
}

func migrateDeployments(store storetypes.KVStore, cdc codec.BinaryCodec) (err error) {
	oStore := prefix.NewStore(store, migrate.DeploymentV1beta3Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.DeploymentFromV1beta3(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		key := dkeeper.MustDeploymentKey(dkeeper.DeploymentStateToPrefix(nVal.State), nVal.ID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}

func migrateGroups(store storetypes.KVStore, cdc codec.BinaryCodec) (err error) {
	oStore := prefix.NewStore(store, migrate.GroupV1beta3Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.GroupFromV1Beta3(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		key := dkeeper.MustGroupKey(dkeeper.GroupStateToPrefix(nVal.State), nVal.ID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}
