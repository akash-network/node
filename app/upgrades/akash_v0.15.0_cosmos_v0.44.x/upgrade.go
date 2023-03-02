// Package akash_v0_15_0_cosmos_v0_44_x
package akash_v0_15_0_cosmos_v0_44_x // nolint revive

import (
	inflationtypes "github.com/akash-network/akash-api/go/node/inflation/v1beta3"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/authz"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	ibcconnectiontypes "github.com/cosmos/ibc-go/v3/modules/core/03-connection/types"
	ibckeeper "github.com/cosmos/ibc-go/v3/modules/core/keeper"

	apptypes "github.com/akash-network/node/app/types"
)

const (
	upgradeName = "akash_v0.15.0_cosmos_v0.44.x"
)

func init() {
	apptypes.RegisterUpgrade(upgradeName, initUpgrade)
}

type upgrade struct {
	*apptypes.App
	ibc *ibckeeper.Keeper
}

var _ apptypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(app *apptypes.App) (apptypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
	}

	val, err := apptypes.FindStructField[*ibckeeper.Keeper](&app.Keepers.Cosmos, "IBC")
	if err != nil {
		return nil, err
	}

	up.ibc = val

	return up, nil
}

func (up *upgrade) StoreLoader() *storetypes.StoreUpgrades {
	upgrades := &storetypes.StoreUpgrades{
		Added: []string{
			authz.ModuleName,
			inflationtypes.ModuleName,
		},
	}

	return upgrades
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, plan upgradetypes.Plan, versionMap module.VersionMap) (module.VersionMap, error) {
		// set max expected block time parameter. Replace the default with your expected value
		up.ibc.ConnectionKeeper.SetParams(ctx, ibcconnectiontypes.DefaultParams())

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

		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}
