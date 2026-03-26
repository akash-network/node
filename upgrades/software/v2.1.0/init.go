// Package v2_1_0
// nolint revive
package v2_1_0

import (
	otypes "pkg.akt.dev/go/node/oracle/v2"

	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)

	utypes.RegisterMigration(otypes.ModuleName, 1, newOracleMigration)
}
