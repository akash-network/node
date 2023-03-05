package app

import (
	"fmt"

	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	apptypes "github.com/akash-network/node/app/types"

	// nolint: revive
	_ "github.com/akash-network/node/app/upgrades/akash_v0.15.0_cosmos_v0.44.x"
	// nolint: revive
	_ "github.com/akash-network/node/app/upgrades/v0.20.0"
)

func (app *AkashApp) registerUpgradeHandlers() error {
	upgradeInfo, err := app.Keepers.Cosmos.Upgrade.ReadUpgradeInfoFromDisk()
	if err != nil {
		return err
	}

	for name, fn := range apptypes.GetUpgradesList() {
		app.Logger().Info(fmt.Sprintf("initializing upgrade `%s`", name))
		upgrade, err := fn(app.Logger(), &app.App)
		if err != nil {
			return fmt.Errorf("unable to unitialize upgrade `%s`: %w", name, err)
		}

		app.Keepers.Cosmos.Upgrade.SetUpgradeHandler(name, upgrade.UpgradeHandler())
		if storeUpgrades := upgrade.StoreLoader(); storeUpgrades != nil && upgradeInfo.Name == name {
			app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, storeUpgrades))
		}
	}

	return nil
}
