// Package v1_0_0
// nolint revive
package v1_0_0

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	utypes "pkg.akt.dev/node/upgrades/types"
)

type takeMigrations struct {
	utypes.Migrator
}

func newTakeMigration(m utypes.Migrator) utypes.Migration {
	return takeMigrations{Migrator: m}
}

func (m takeMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates provider store from version 2 to 3.
func (m takeMigrations) handler(_ sdk.Context) error {
	return nil
}
