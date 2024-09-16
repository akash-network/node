// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	"pkg.akt.dev/go/node/migrate"
	types "pkg.akt.dev/go/node/provider/v1beta4"
	"pkg.akt.dev/go/sdkutil"

	utypes "pkg.akt.dev/node/upgrades/types"
	pkeeper "pkg.akt.dev/node/x/provider/keeper"
)

type providerMigrations struct {
	utypes.Migrator
}

func newProviderMigration(m utypes.Migrator) utypes.Migration {
	return providerMigrations{Migrator: m}
}

func (m providerMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates provider store from version 2 to 3.
func (m providerMigrations) handler(ctx sdk.Context) (err error) {
	store := ctx.KVStore(m.StoreKey())
	pstore := prefix.NewStore(store, types.ProviderPrefix())

	iter := store.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	cdc := m.Codec()

	for ; iter.Valid(); iter.Next() {
		to := migrate.ProviderFromV1beta3(cdc, iter.Value())

		id := sdkutil.MustAccAddressFromBech32(to.Owner)
		bz := cdc.MustMarshal(&to)

		store.Delete(iter.Key())
		pstore.Set(pkeeper.ProviderKey(id), bz)
	}

	return nil
}
