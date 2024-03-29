// Package v0_32_0
// nolint revive
package v0_32_0

import (
	mv1beta4 "github.com/akash-network/akash-api/go/node/market/v1beta4"

	utypes "github.com/akash-network/node/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)
	utypes.RegisterMigration(mv1beta4.ModuleName, 4, newMarketMigration)
}
