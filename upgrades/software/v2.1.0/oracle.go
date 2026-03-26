package v2_1_0

import (
	"bytes"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

type oracleMigration struct {
	utypes.Migrator
}

func newOracleMigration(m utypes.Migrator) utypes.Migration {
	return oracleMigration{Migrator: m}
}

func (m oracleMigration) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates oracle from version 1 to 2 by wiping all state.
// After migrations run, the upgrade handler re-initializes params and sources.
func (m oracleMigration) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	// Collect all keys first (cannot delete while iterating)
	var keys [][]byte
	iter := store.Iterator(nil, nil)
	for iter.Valid() {
		keys = append(keys, bytes.Clone(iter.Key()))
		iter.Next()
	}
	if err := iter.Close(); err != nil {
		return err
	}

	for _, key := range keys {
		store.Delete(key)
	}

	return nil
}
