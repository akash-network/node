// Package v0_15_0
// nolint revive
package v0_15_0

import (
	av1beta2 "github.com/akash-network/akash-api/go/node/audit/v1beta2"
	cv1beta2 "github.com/akash-network/akash-api/go/node/cert/v1beta2"
	dv1beta2 "github.com/akash-network/akash-api/go/node/deployment/v1beta2"
	ev1beta2 "github.com/akash-network/akash-api/go/node/escrow/v1beta2"
	mv1beta2 "github.com/akash-network/akash-api/go/node/market/v1beta2"
	pv1beta2 "github.com/akash-network/akash-api/go/node/provider/v1beta2"

	"github.com/akash-network/node/migrations/consensus"
)

func init() {
	consensus.RegisterMigration(av1beta2.ModuleName, 1, newAuditMigration)
	consensus.RegisterMigration(cv1beta2.ModuleName, 1, newCertMigration)
	consensus.RegisterMigration(dv1beta2.ModuleName, 1, newDeploymentMigration)
	consensus.RegisterMigration(mv1beta2.ModuleName, 1, newMarketMigration)
	consensus.RegisterMigration(pv1beta2.ModuleName, 1, newProviderMigration)
	consensus.RegisterMigration(ev1beta2.ModuleName, 1, newEscrowMigration)
}
