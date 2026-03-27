package v2_1_0

import (
	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	dv1 "pkg.akt.dev/go/node/deployment/v1"

	utypes "pkg.akt.dev/node/v2/upgrades/types"
	"pkg.akt.dev/node/v2/x/deployment/keeper/keys"
)

type deploymentMigrations struct {
	utypes.Migrator
}

func newDeploymentMigration(m utypes.Migrator) utypes.Migration {
	return deploymentMigrations{Migrator: m}
}

func (m deploymentMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates deployment from version 7 to 8.
func (m deploymentMigrations) handler(sctx sdk.Context) error {
	skey := m.StoreKey().(*storetypes.KVStoreKey)
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	pendingDenomMigrations := collections.NewMap(sb, collections.NewPrefix(keys.PendingDenomMigrationPrefix), "pending_denom_migrations", keys.DeploymentPrimaryKeyCodec, sdk.IntValue)

	_, err := sb.Build()
	if err != nil {
		return err
	}

	var dids []dv1.DeploymentID

	err = pendingDenomMigrations.Walk(sctx, nil, func(pk keys.DeploymentPrimaryKey, value sdkmath.Int) (stop bool, err error) {
		dids = append(dids, keys.KeyToDeploymentID(pk))
		return false, nil
	})
	if err != nil {
		return err
	}

	for _, did := range dids {
		err = pendingDenomMigrations.Remove(sctx, keys.DeploymentIDToKey(did))
		if err != nil {
			return err
		}
	}

	return nil
}
