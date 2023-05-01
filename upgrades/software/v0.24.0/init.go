// Package v0_24_0
// nolint revive
package v0_24_0

import (
	dv1beta3 "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	mv1beta3 "github.com/akash-network/akash-api/go/node/market/v1beta3"

	utypes "github.com/akash-network/node/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(upgradeName, initUpgrade)

	utypes.RegisterMigration(dv1beta3.ModuleName, 2, newDeploymentMigration)
	utypes.RegisterMigration(mv1beta3.ModuleName, 2, newMarketMigration)
}
