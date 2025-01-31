// Package v0_32_0
// nolint revive
package v0_32_0

import (
	types "github.com/akash-network/akash-api/go/node/market/v1beta4"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	utypes "github.com/akash-network/node/upgrades/types"
	"github.com/akash-network/node/x/market/keeper/keys/v1beta4"
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

// handler migrates market from version 3 to 4.
func (m marketMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())
	iter := sdk.KVStorePrefixIterator(store, types.LeasePrefix())
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.Lease
		m.Codec().MustUnmarshal(iter.Value(), &val)

		store.Delete(v1beta4.SecondaryLeaseKeyByProviderLegacy(val.LeaseID))
	}

	return nil
}
