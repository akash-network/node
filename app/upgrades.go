package app

import (
	"fmt"

	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	utypes "github.com/akash-network/node/upgrades/types"
	// nolint: revive
	_ "github.com/akash-network/node/upgrades"
)

func (app *AkashApp) registerUpgradeHandlers() error {
	upgradeInfo, err := app.Keepers.Cosmos.Upgrade.ReadUpgradeInfoFromDisk()
	if err != nil {
		return err
	}

	for name, fn := range utypes.GetUpgradesList() {
		app.Logger().Info(fmt.Sprintf("initializing upgrade `%s`", name))
		upgrade, err := fn(app.Logger(), &app.App)
		if err != nil {
			return fmt.Errorf("unable to unitialize upgrade `%s`: %w", name, err)
		}

		app.Keepers.Cosmos.Upgrade.SetUpgradeHandler(name, upgrade.UpgradeHandler())
		if storeUpgrades := upgrade.StoreLoader(); storeUpgrades != nil && upgradeInfo.Name == name {
			app.Logger().Info(fmt.Sprintf("applying store upgrades for `%s`", name))
			app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, storeUpgrades))
		}
	}

	return nil
}
