// Package v1_0_0
// nolint revive
package v1_0_0

import (
	av1 "pkg.akt.dev/go/node/audit/v1"
	cv1 "pkg.akt.dev/go/node/cert/v1"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1beta5"
	pv1 "pkg.akt.dev/go/node/provider/v1beta4"
	tv1 "pkg.akt.dev/go/node/take/v1"

	utypes "pkg.akt.dev/akashd/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)

	utypes.RegisterMigration(av1.ModuleName, 2, newAuditMigration)
	utypes.RegisterMigration(cv1.ModuleName, 2, newCertsMigration)
	utypes.RegisterMigration(dv1.ModuleName, 3, newDeploymentsMigration)
	utypes.RegisterMigration(ev1.ModuleName, 2, newEscrowMigration)
	utypes.RegisterMigration(mv1.ModuleName, 4, newMarketMigration)
	utypes.RegisterMigration(pv1.ModuleName, 2, newProviderMigration)
	utypes.RegisterMigration(tv1.ModuleName, 2, newTakeMigration)
}
