// Package v0_20_0
package v0_20_0 // nolint revive

import (
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	icacontrollertypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/controller/types"
	icahosttypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/types"

	apptypes "github.com/akash-network/node/app/types"
)

const (
	upgradeName = "v0.20.0"
)

func init() {
	apptypes.RegisterUpgrade(upgradeName, initUpgrade)
}

type upgrade struct {
	*apptypes.App
}

var _ apptypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(app *apptypes.App) (apptypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
	}

	return up, nil
}

func (up *upgrade) StoreLoader() *storetypes.StoreUpgrades {
	upgrades := &storetypes.StoreUpgrades{
		Deleted: []string{
			icacontrollertypes.StoreKey,
			icahosttypes.StoreKey,
		},
	}

	return upgrades
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		ctx.Logger().Info("start to roll-back interchainaccount module...")

		delete(fromVM, icatypes.ModuleName)

		ctx.Logger().Info("start to run module migrations...")

		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}
