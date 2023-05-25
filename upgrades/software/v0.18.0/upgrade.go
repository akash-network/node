// Package v0_18_0
// nolint revive
package v0_18_0

import (
	"fmt"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	ica "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts"
	icacontrollertypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/controller/types"
	icahosttypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/types"
	"github.com/tendermint/tendermint/libs/log"

	apptypes "github.com/akash-network/node/app/types"
	utypes "github.com/akash-network/node/upgrades/types"
)

const (
	upgradeName = "v0.18.0"
)

type upgrade struct {
	*apptypes.App
}

var _ utypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(_ log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
	}

	if _, exists := up.MM.Modules[icatypes.ModuleName]; !exists {
		return nil, fmt.Errorf("module %s has not been initialized", icatypes.ModuleName) // nolint: goerr113
	}

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
		fromVM[icatypes.ModuleName] = up.MM.Modules[icatypes.ModuleName].ConsensusVersion()

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
		up.MM.Modules[icatypes.ModuleName].(ica.AppModule).InitModule(ctx, controllerParams, hostParams)

		ctx.Logger().Info("start to run module migrations...")

		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}
