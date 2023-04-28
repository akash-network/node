// Package v0_15_0
// nolint revive
package v0_15_0

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	v043 "github.com/cosmos/cosmos-sdk/x/distribution/legacy/v043"

	cv1beta2 "github.com/akash-network/akash-api/go/node/cert/v1beta2"

	"github.com/akash-network/node/migrations/consensus"
)

type certMigrations struct {
	consensus.Migrator
}

func newCertMigration(m consensus.Migrator) consensus.Migration {
	return certMigrations{Migrator: m}
}

func (m certMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates provider from version 1 to 2.
func (m certMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	v043.MigratePrefixAddressAddress(store, cv1beta2.PrefixCertificateID())

	return nil
}
