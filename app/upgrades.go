package app

import (
	"fmt"

	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	utypes "pkg.akt.dev/akashd/upgrades/types"
	// nolint: revive
	_ "pkg.akt.dev/akashd/upgrades"
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
		// app.customPreUpgradeHandler(upgradeInfo)
	}

	for name, fn := range utypes.GetUpgradesList() {
		app.Logger().Info(fmt.Sprintf("configuring upgrade `%s`", name))
		upgrade, err := fn(app.Logger(), &app.App)
		if err != nil {
			return fmt.Errorf("unable to unitialize upgrade `%s`: %w", name, err)
		}

		app.Keepers.Cosmos.Upgrade.SetUpgradeHandler(name, upgrade.UpgradeHandler())

		if upgradeInfo.Name == name {
			if storeUpgrades := upgrade.StoreLoader(); storeUpgrades != nil && upgradeInfo.Name == name {
				app.Logger().Info(fmt.Sprintf("setting up store upgrades for `%s`", name))
				app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, storeUpgrades))
			}
		}
	}

	return nil
}
