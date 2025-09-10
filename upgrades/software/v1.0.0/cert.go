// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	"pkg.akt.dev/go/node/migrate"

	utypes "pkg.akt.dev/node/upgrades/types"
	ckeeper "pkg.akt.dev/node/x/cert/keeper"
)

type certsMigrations struct {
	utypes.Migrator
}

func newCertsMigration(m utypes.Migrator) utypes.Migration {
	return certsMigrations{Migrator: m}
}

func (m certsMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates certificates store from version 2 to 3.
func (m certsMigrations) handler(ctx sdk.Context) (err error) {
	cdc := m.Codec()

	store := ctx.KVStore(m.StoreKey())
	oStore := prefix.NewStore(store, migrate.CertV1beta3Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		val := migrate.CertFromV1beta3(cdc, iter.Value())

		id, err := ckeeper.ParseCertID(nil, iter.Key())
		if err != nil {
			return err
		}

		bz := cdc.MustMarshal(&val)
		key := ckeeper.MustCertificateKey(val.State, id)
		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}
