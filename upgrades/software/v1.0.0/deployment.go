// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"fmt"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta "pkg.akt.dev/go/node/deployment/v1beta4"
	"pkg.akt.dev/go/node/migrate"

	utypes "pkg.akt.dev/node/upgrades/types"
	dkeeper "pkg.akt.dev/node/x/deployment/keeper"
)

type deploymentsMigrations struct {
	utypes.Migrator
}

func newDeploymentsMigration(m utypes.Migrator) utypes.Migration {
	return deploymentsMigrations{Migrator: m}
}

func (m deploymentsMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates deployment store from version 4 to 5
func (m deploymentsMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	// deployment prefix does not change in this upgrade
	oStore := prefix.NewStore(store, dkeeper.DeploymentPrefix)

	iter := oStore.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	var deploymentsTotal uint64
	var deploymentsActive uint64
	var deploymentsClosed uint64

	cdc := m.Codec()

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.DeploymentFromV1beta3(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		switch nVal.State {
		case dv1.DeploymentActive:
			deploymentsActive++
		case dv1.DeploymentClosed:
			deploymentsClosed++
		default:
			return fmt.Errorf("unknown order state %d", nVal.State)
		}

		deploymentsTotal++

		key := dkeeper.MustDeploymentKey(dkeeper.DeploymentStateToPrefix(nVal.State), nVal.ID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	// group prefix does not change in this upgrade
	oStore = prefix.NewStore(store, dkeeper.GroupPrefix)

	iter = oStore.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	var groupsTotal uint64
	var groupsOpen uint64
	var groupsPaused uint64
	var groupsInsufficientFunds uint64
	var groupsClosed uint64

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.GroupFromV1Beta3(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		switch nVal.State {
		case dv1beta.GroupOpen:
			groupsOpen++
		case dv1beta.GroupPaused:
			groupsPaused++
		case dv1beta.GroupInsufficientFunds:
			groupsInsufficientFunds++
		case dv1beta.GroupClosed:
			groupsClosed++
		default:
			return fmt.Errorf("unknown order state %d", nVal.State)
		}

		groupsTotal++

		key := dkeeper.MustGroupKey(dkeeper.GroupStateToPrefix(nVal.State), nVal.ID)

		oStore.Delete(iter.Key())
		store.Set(key, bz)
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
