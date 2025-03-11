// Package v0_38_0
// nolint revive
package v0_38_0

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	utypes "github.com/akash-network/node/upgrades/types"

	"github.com/cosmos/cosmos-sdk/x/authz/keeper"
)

type authzMigrations struct {
	utypes.Migrator
}

func newAuthzMigration(m utypes.Migrator) utypes.Migration {
	return authzMigrations{Migrator: m}
}

func (m authzMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates authz from version 1 to 2.
func (m authzMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	iter := sdk.KVStorePrefixIterator(store, keeper.GrantKey)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		granter, grantee := keeper.AddressesFromGrantStoreKey(iter.Key())
		keeper.IncGranteeGrants(store, grantee, granter)
	}

	return nil
}
