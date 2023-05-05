// Package v0_15_0
// nolint revive
package v0_15_0

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	v043 "github.com/cosmos/cosmos-sdk/x/distribution/legacy/v043"

	av1beta2 "github.com/akash-network/akash-api/go/node/audit/v1beta2"

	utypes "github.com/akash-network/node/upgrades/types"
)

type auditMigrations struct {
	utypes.Migrator
}

func newAuditMigration(m utypes.Migrator) utypes.Migration {
	return auditMigrations{Migrator: m}
}

func (m auditMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates provider from version 1 to 2.
func (m auditMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	v043.MigratePrefixAddressAddress(store, av1beta2.PrefixProviderID())

	return nil
}
