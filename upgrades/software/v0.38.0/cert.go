// Package v0_38_0
// nolint revive
package v0_38_0

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"

	utypes "github.com/akash-network/node/upgrades/types"
	"github.com/akash-network/node/x/cert/keeper"
)

type certMigrations struct {
	utypes.Migrator
}

func newCertMigration(m utypes.Migrator) utypes.Migration {
	return certMigrations{Migrator: m}
}

func (m certMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates x/cert from version 2 to 3.
func (m certMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	iter := sdk.KVStorePrefixIterator(store, types.PrefixCertificateID())

	defer func() {
		_ = iter.Close()
	}()

	var total int

	for ; iter.Valid(); iter.Next() {
		id, err := keeper.ParseCertIDLegacy(types.PrefixCertificateID(), iter.Key())
		if err != nil {
			return err
		}

		store.Delete(iter.Key())

		key := keeper.CertificateKey(id)
		store.Set(key, iter.Value())

		total++
	}

	ctx.Logger().Info(fmt.Sprintf("[upgrade %s]: updated x/cert store keys. total=%d", UpgradeName, total))

	return nil
}
