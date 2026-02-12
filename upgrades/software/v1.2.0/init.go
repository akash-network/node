// Package v1_2_0
// nolint revive
package v1_2_0

import (
	mv1 "pkg.akt.dev/go/node/market/v1"

	utypes "pkg.akt.dev/node/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)

	utypes.RegisterMigration(mv1.ModuleName, 7, newMarketMigration)
}
