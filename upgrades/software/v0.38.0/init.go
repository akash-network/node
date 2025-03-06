// Package v0_38_0
// nolint revive
package v0_38_0

import (
	ctypesbeta "github.com/akash-network/akash-api/go/node/cert/v1beta3"
	dv1beta3 "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	mtypesbeta "github.com/akash-network/akash-api/go/node/market/v1beta4"

	"github.com/cosmos/cosmos-sdk/x/authz"

	utypes "github.com/akash-network/node/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)
	utypes.RegisterMigration(ctypesbeta.ModuleName, 2, newCertMigration)
	utypes.RegisterMigration(mtypesbeta.ModuleName, 5, newMarketMigration)
	utypes.RegisterMigration(dv1beta3.ModuleName, 3, newDeploymentMigration)
	utypes.RegisterMigration(authz.ModuleName, 1, newAuthzMigration)
}
