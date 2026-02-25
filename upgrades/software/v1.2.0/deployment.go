// Package v1_2_0
// nolint revive
package v1_2_0

import (
	"fmt"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"

	utypes "pkg.akt.dev/node/upgrades/types"
	dkeeper "pkg.akt.dev/node/x/deployment/keeper"
	dkeys "pkg.akt.dev/node/x/deployment/keeper/keys"
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

// handler migrates deployment from version 5 to 6.
// Moves deployments and groups from manual KVStore keys to collections.IndexedMap.
func (m deploymentMigrations) handler(ctx sdk.Context) error {
	skey := m.StoreKey()
	cdc := m.Codec()
	store := ctx.KVStore(skey)

	// Build IndexedMaps locally (same construction as NewKeeper)
	ssvc := runtime.NewKVStoreService(skey.(*storetypes.KVStoreKey))
	sb := collections.NewSchemaBuilder(ssvc)

	deploymentIndexes := dkeeper.NewDeploymentIndexes(sb)
	deployments := collections.NewIndexedMap(sb, collections.NewPrefix(dkeys.DeploymentPrefixNew), "deployments", dkeys.DeploymentPrimaryKeyCodec, codec.CollValue[dv1.Deployment](cdc), deploymentIndexes)

	groupIndexes := dkeeper.NewGroupIndexes(sb)
	groups := collections.NewIndexedMap(sb, collections.NewPrefix(dkeys.GroupPrefixNew), "groups", dkeys.GroupPrimaryKeyCodec, codec.CollValue[dtypes.Group](cdc), groupIndexes)

	if _, err := sb.Build(); err != nil {
		return err
	}

	// === Deployments ===
	diter := storetypes.KVStorePrefixIterator(store, dkeys.DeploymentPrefix)
	defer func() {
		_ = diter.Close()
	}()

	var deploymentCount int64
	var groupCount int64

	for ; diter.Valid(); diter.Next() {
		var deployment dv1.Deployment
		cdc.MustUnmarshal(diter.Value(), &deployment)

		pk := dkeys.DeploymentIDToKey(deployment.ID)
		if err := deployments.Set(ctx, pk, deployment); err != nil {
			return fmt.Errorf("failed to migrate deployment %s: %w", deployment.ID, err)
		}

		store.Delete(diter.Key())

		deploymentCount++
	}

	// === Groups ===
	giter := storetypes.KVStorePrefixIterator(store, dkeys.GroupPrefix)
	defer func() {
		_ = giter.Close()
	}()

	for ; giter.Valid(); giter.Next() {
		var group dtypes.Group
		cdc.MustUnmarshal(giter.Value(), &group)

		pk := dkeys.GroupIDToKey(group.ID)
		if err := groups.Set(ctx, pk, group); err != nil {
			return fmt.Errorf("failed to migrate group %s: %w", group.ID, err)
		}

		store.Delete(giter.Key())

		groupCount++
	}

	ctx.Logger().Info("deployment store migration complete",
		"deployments_migrated", deploymentCount,
		"groups_migrated", groupCount,
	)

	return nil
}
