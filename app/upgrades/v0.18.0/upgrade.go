// Package v0_18_0
package v0_18_0 // nolint revive

import (
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	ica "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts"
	icacontrollertypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/controller/types"
	icahosttypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/types"

	apptypes "github.com/akash-network/node/app/types"
)

const (
	upgradeName = "v0.18.0"
)

func init() {
	apptypes.RegisterUpgrade(upgradeName, initUpgrade)
}

type upgrade struct {
	*apptypes.App
	icaModule ica.AppModule
}

var _ apptypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(app *apptypes.App) (apptypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
	}

	val, err := apptypes.FindStructField[ica.AppModule](&app.Modules.Cosmos, "ICAModule")
	if err != nil {
		return nil, err
	}

	up.icaModule = val

	return up, nil
}

func (up *upgrade) StoreLoader() *storetypes.StoreUpgrades {
	upgrades := &storetypes.StoreUpgrades{
		Added: []string{
			icacontrollertypes.StoreKey,
			icahosttypes.StoreKey,
		},
	}

	return upgrades
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		fromVM[icatypes.ModuleName] = up.icaModule.ConsensusVersion()

		// create ICS27 Controller submodule params
		// enable the controller chain
		controllerParams := icacontrollertypes.Params{ControllerEnabled: true}

		// create ICS27 Host submodule params
		hostParams := icahosttypes.Params{
			// enable the host chain
			HostEnabled: true,
			// allowing the all messages
			AllowMessages: []string{"*"},
		}

		ctx.Logger().Info("start to init interchainaccount module...")
		// initialize ICS27 module
		up.icaModule.InitModule(ctx, controllerParams, hostParams)

		ctx.Logger().Info("start to run module migrations...")

		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}
