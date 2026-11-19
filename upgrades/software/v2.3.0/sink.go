package v2_3_0

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

type sinkMigration struct {
	utypes.Migrator
}

func newSinkMigration(m utypes.Migrator) utypes.Migration {
	return sinkMigration{Migrator: m}
}

func (m sinkMigration) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

func (m sinkMigration) handler(ctx sdk.Context) error {
	return nil
}
