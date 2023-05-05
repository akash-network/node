// Package v0_15_0
// nolint revive
package v0_15_0

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	utypes "github.com/akash-network/node/upgrades/types"
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

// handler migrates provider from version 1 to 2.
func (m providerMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	// old key is of format:
	// ownerAddrBytes (20 bytes)
	// new key is of format
	// ownerAddrLen (1 byte) || ownerAddrBytes
	oldStoreIter := store.Iterator(nil, nil)
	defer func() {
		_ = oldStoreIter.Close()
	}()

	for ; oldStoreIter.Valid(); oldStoreIter.Next() {
		newStoreKey := address.MustLengthPrefix(oldStoreIter.Key())

		// Set new key on store. Values don't change.
		store.Set(newStoreKey, oldStoreIter.Value())
		store.Delete(oldStoreIter.Key())
	}
	return nil
}
