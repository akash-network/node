package app

import (
	"fmt"

	upgradetypes "cosmossdk.io/x/upgrade/types"

	utypes "pkg.akt.dev/node/upgrades/types"
	// nolint: revive
	_ "pkg.akt.dev/node/upgrades"
)

func (app *AkashApp) registerUpgradeHandlers() error {
	upgradeInfo, err := app.Keepers.Cosmos.Upgrade.ReadUpgradeInfoFromDisk()
	if err != nil {
		return err
	}

	if app.Keepers.Cosmos.Upgrade.IsSkipHeight(upgradeInfo.Height) {
		return nil
	}

	currentHeight := app.CommitMultiStore().LastCommitID().Version

	if upgradeInfo.Height == currentHeight+1 {
		app.customPreUpgradeHandler(upgradeInfo)
	}

	for name, fn := range utypes.GetUpgradesList() {
		upgrade, err := fn(app.Logger(), app.App)
		if err != nil {
			return fmt.Errorf("unable to unitialize upgrade `%s`: %w", name, err)
		}

		app.Keepers.Cosmos.Upgrade.SetUpgradeHandler(name, upgrade.UpgradeHandler())

		if upgradeInfo.Name == name {
			app.Logger().Info(fmt.Sprintf("configuring upgrade `%s`", name))
			if storeUpgrades := upgrade.StoreLoader(); storeUpgrades != nil && upgradeInfo.Name == name {
				app.Logger().Info(fmt.Sprintf("setting up store upgrades for `%s`", name))
				app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, storeUpgrades))
			}
		}
	}

	utypes.IterateMigrations(func(module string, version uint64, initfn utypes.NewMigrationFn) {
		migrator := initfn(utypes.NewMigrator(app.cdc, app.GetKey(module)))
		if err := app.Configurator.RegisterMigration(module, version, migrator.GetHandler()); err != nil {
			panic(err)
		}
	})

	return nil
}

func (app *AkashApp) customPreUpgradeHandler(_ upgradetypes.Plan) {
}
