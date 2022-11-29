package app

import (
	bam "github.com/cosmos/cosmos-sdk/baseapp"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/authz"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	ica "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts"
	icacontrollertypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/controller/types"
	icahosttypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/types"
	ibcconnectiontypes "github.com/cosmos/ibc-go/v3/modules/core/03-connection/types"

	icaauthtypes "github.com/ovrclk/akash/x/icaauth/types/v1beta2"
	inflationtypes "github.com/ovrclk/akash/x/inflation/types/v1beta2"
)

type storeLoaderFn func(int64) bam.StoreLoader

type upgradeFuncs struct {
	handler     upgradetypes.UpgradeHandler
	storeLoader storeLoaderFn
}

type upgradeHandlers map[string]upgradeFuncs

func (app *AkashApp) loadUpgradeHandlers(icaModule ica.AppModule) upgradeHandlers {
	handlers := make(map[string]upgradeFuncs)

	handlers["v0.20.0"] = upgradeFuncs{
		handler:     app.upgrade_v0_20_0(icaModule),
		storeLoader: storeLoader_v0_20_0,
	}

	handlers["v0.18.0"] = upgradeFuncs{
		handler:     app.upgrade_v0_18_0(icaModule),
		storeLoader: storeLoader_v0_18_0,
	}

	handlers["akash_v0.15.0_cosmos_v0.44.x"] = upgradeFuncs{
		handler:     app.upgrade_akash_v0_15_0_cosmos_v0_44_x(),
		storeLoader: storeLoader_akash_v0_15_0_cosmos_v0_44_x,
	}

	return handlers
}

// upgradeDefaultHandler creates an SDK upgrade handler for v4
func (app *AkashApp) upgradeDefaultHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, _ upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
		newVm, err := app.mm.RunMigrations(ctx, app.configurator, vm)
		if err != nil {
			return newVm, err
		}
		return newVm, nil
	}
}

// storeLoader_v0_20_0 fixes pruning issue introduced in the v0.18.0 upgrade due to missing icaauthtypes.StoreKey
// it deletes previously added store keys and adds them again with icaauthtypes.StoreKey included
// nolint: revive
func storeLoader_v0_20_0(upgradeHeight int64) bam.StoreLoader {
	return func(ms sdk.CommitMultiStore) error {
		if upgradeHeight == ms.LastCommitID().Version+1 {
			err := ms.LoadLatestVersionAndUpgrade(&storetypes.StoreUpgrades{
				Deleted: []string{
					icacontrollertypes.StoreKey,
					icahosttypes.StoreKey,
				},
			})
			if err != nil {
				return err
			}
			err = ms.LoadLatestVersionAndUpgrade(&storetypes.StoreUpgrades{
				Added: []string{
					icacontrollertypes.StoreKey,
					icahosttypes.StoreKey,
					icaauthtypes.StoreKey,
				},
			})
			if err != nil {
				return err
			}
			return nil
		}

		// Otherwise load default store loader
		return bam.DefaultStoreLoader(ms)
	}
}

// nolint: revive
func (app *AkashApp) upgrade_v0_20_0(icaModule ica.AppModule) upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		fromVM[icatypes.ModuleName] = icaModule.ConsensusVersion()

		// create ICS27 Controller submodule params
		// disable the controller chain
		controllerParams := icacontrollertypes.Params{
			ControllerEnabled: false,
		}

		// create ICS27 Host submodule params
		hostParams := icahosttypes.Params{
			// disable the host chain
			HostEnabled:   false,
			AllowMessages: []string{},
		}

		ctx.Logger().Info("start re-init interchainaccount module...")
		// initialize ICS27 module
		icaModule.InitModule(ctx, controllerParams, hostParams)

		ctx.Logger().Info("start to run module migrations...")

		return app.mm.RunMigrations(ctx, app.configurator, fromVM)
	}
}

// nolint: revive
func storeLoader_v0_18_0(upgradeHeight int64) bam.StoreLoader {
	storeUpgrades := &storetypes.StoreUpgrades{
		Added: []string{
			icacontrollertypes.StoreKey,
			icahosttypes.StoreKey,
		},
	}

	return upgradetypes.UpgradeStoreLoader(upgradeHeight, storeUpgrades)
}

// nolint: revive
func (app *AkashApp) upgrade_v0_18_0(icaModule ica.AppModule) upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		fromVM[icatypes.ModuleName] = icaModule.ConsensusVersion()

		// create ICS27 Controller submodule params
		// enable the controller chain
		controllerParams := icacontrollertypes.Params{
			ControllerEnabled: true,
		}

		// create ICS27 Host submodule params
		hostParams := icahosttypes.Params{
			// enable the host chain
			HostEnabled: true,
			// allowing the all messages
			AllowMessages: []string{"*"},
		}

		ctx.Logger().Info("start to init interchainaccount module...")
		// initialize ICS27 module
		icaModule.InitModule(ctx, controllerParams, hostParams)

		ctx.Logger().Info("start to run module migrations...")

		return app.mm.RunMigrations(ctx, app.configurator, fromVM)
	}
}

// nolint: revive
func storeLoader_akash_v0_15_0_cosmos_v0_44_x(upgradeHeight int64) bam.StoreLoader {
	storeUpgrades := &storetypes.StoreUpgrades{
		Added: []string{
			authz.ModuleName,
			inflationtypes.ModuleName,
		},
	}

	return upgradetypes.UpgradeStoreLoader(upgradeHeight, storeUpgrades)
}

// nolint: revive
func (app *AkashApp) upgrade_akash_v0_15_0_cosmos_v0_44_x() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, plan upgradetypes.Plan, versionMap module.VersionMap) (module.VersionMap, error) {
		// set max expected block time parameter. Replace the default with your expected value
		app.Keeper.IBC.ConnectionKeeper.SetParams(ctx, ibcconnectiontypes.DefaultParams())

		// 1st-time running in-store migrations, using 1 as fromVersion to
		// avoid running InitGenesis.
		fromVM := map[string]uint64{
			"auth":         1,
			"bank":         1,
			"capability":   1,
			"crisis":       1,
			"distribution": 1,
			"evidence":     1,
			"gov":          1,
			"mint":         1,
			"params":       1,
			"slashing":     1,
			"staking":      1,
			"upgrade":      1,
			"vesting":      1,
			"ibc":          1,
			"genutil":      1,
			"transfer":     1,

			// akash modules
			"audit":      1,
			"cert":       1,
			"deployment": 1,
			"escrow":     1,
			"market":     1,
			"provider":   1,
		}

		return app.mm.RunMigrations(ctx, app.configurator, fromVM)
	}
}
