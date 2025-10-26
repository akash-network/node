// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	"pkg.akt.dev/go/node/migrate"
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

func ProviderKey(id sdk.Address) []byte {
	return address.MustLengthPrefix(id.Bytes())
}

// handler migrates provider store from version 2 to 3.
func (m providerMigrations) handler(ctx sdk.Context) (err error) {
	store := ctx.KVStore(m.StoreKey())

	iter := store.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	cdc := m.Codec()

	var providersTotal uint64

	for ; iter.Valid(); iter.Next() {
		to := migrate.ProviderFromV1beta3(cdc, iter.Value())

		id := sdkutil.MustAccAddressFromBech32(to.Owner)
		bz := cdc.MustMarshal(&to)

		providersTotal++

		store.Delete(iter.Key())
		store.Set(pkeeper.ProviderKey(id), bz)
	}

	ctx.Logger().Info(fmt.Sprintf("[upgrade %s]: updated x/provider store keys:"+
		"\n\tproviders total: %d",
		UpgradeName,
		providersTotal))

	return nil
}
