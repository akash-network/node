// Package v2_0_0
// nolint revive
package v2_0_0

import (
	dv1 "pkg.akt.dev/go/node/deployment/v1"

	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)

	utypes.RegisterMigration(dv1.ModuleName, 6, newDeploymentMigration)
}
