// Package v0_38_0
// nolint revive
package v0_38_0

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	dtypesbeta "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	utypes "github.com/akash-network/node/upgrades/types"
	"github.com/akash-network/node/x/deployment/keeper"
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

// handler migrates deployment from version 2 to 3.
func (m deploymentMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())
	diter := sdk.KVStorePrefixIterator(store, dtypesbeta.DeploymentPrefix())

	defer func() {
		_ = diter.Close()
	}()

	var deploymentsTotal uint64
	var deploymentsActive uint64
	var deploymentsClosed uint64

	for ; diter.Valid(); diter.Next() {
		var val dtypesbeta.Deployment
		m.Codec().MustUnmarshal(diter.Value(), &val)

		switch val.State {
		case dtypesbeta.DeploymentActive:
			deploymentsActive++
		case dtypesbeta.DeploymentClosed:
			deploymentsClosed++
		default:
			return fmt.Errorf("[upgrade %s]: unknown deployment state %d", UpgradeName, val.State)
		}

		key, err := keeper.DeploymentKey(keeper.DeploymentStateToPrefix(val.State), val.DeploymentID)
		if err != nil {
			return err
		}

		data, err := m.Codec().Marshal(&val)
		if err != nil {
			return err
		}

		store.Delete(keeper.DeploymentKeyLegacy(val.DeploymentID))
		store.Set(key, data)

		deploymentsTotal++
	}

	giter := sdk.KVStorePrefixIterator(store, dtypesbeta.GroupPrefix())

	defer func() {
		_ = giter.Close()
	}()

	var groupsTotal uint64
	var groupsOpen uint64
	var groupsPaused uint64
	var groupsInsufficientFunds uint64
	var groupsClosed uint64

	for ; giter.Valid(); giter.Next() {
		var val dtypesbeta.Group
		m.Codec().MustUnmarshal(giter.Value(), &val)

		switch val.State {
		case dtypesbeta.GroupOpen:
			groupsOpen++
		case dtypesbeta.GroupPaused:
			groupsPaused++
		case dtypesbeta.GroupInsufficientFunds:
			groupsInsufficientFunds++
		case dtypesbeta.GroupClosed:
			groupsClosed++
		default:
			return fmt.Errorf("[upgrade %s]: unknown deployment group state %d", UpgradeName, val.State)
		}

		key, err := keeper.GroupKey(keeper.GroupStateToPrefix(val.State), val.GroupID)
		if err != nil {
			return err
		}

		data, err := m.Codec().Marshal(&val)
		if err != nil {
			return err
		}

		store.Delete(keeper.GroupKeyLegacy(val.GroupID))
		store.Set(key, data)

		groupsTotal++
	}

	ctx.Logger().Info(fmt.Sprintf("[upgrade %s]: updated x/deployment store keys:"+
		"\n\tdeployments total:              %d"+
		"\n\tdeployments active:             %d"+
		"\n\tdeployments closed:             %d"+
		"\n\tgroups total:                   %d"+
		"\n\tgroups open:                    %d"+
		"\n\tgroups paused:                  %d"+
		"\n\tgroups insufficient funds:      %d"+
		"\n\tgroups closed:                  %d",
		UpgradeName,
		deploymentsTotal, deploymentsActive, deploymentsClosed,
		groupsTotal, groupsOpen, groupsPaused, groupsInsufficientFunds, groupsClosed))

	return nil
}
