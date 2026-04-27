// Package v2_1_0
// nolint revive
package v2_1_0

import (
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	otypes "pkg.akt.dev/go/node/oracle/v2"

	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)

	utypes.RegisterMigration(otypes.ModuleName, 1, newOracleMigration)
	utypes.RegisterMigration(dv1.ModuleName, 7, newDeploymentMigration)
	utypes.RegisterMigration(mv1.ModuleName, 8, newMarketMigration)
}
