// Package v0_15_0
// nolint revive
package v0_15_0

import (
	inflationtypes "github.com/akash-network/akash-api/go/node/inflation/v1beta3"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/authz"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	ibcconnectiontypes "github.com/cosmos/ibc-go/v4/modules/core/03-connection/types"
	ibckeeper "github.com/cosmos/ibc-go/v4/modules/core/keeper"
	"github.com/tendermint/tendermint/libs/log"

	apptypes "github.com/akash-network/node/app/types"
	utypes "github.com/akash-network/node/upgrades/types"
)

const (
	upgradeName = "akash_v0.15.0_cosmos_v0.44.x"
)

type upgrade struct {
	*apptypes.App
	ibc *ibckeeper.Keeper
}

var _ utypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(_ log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
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
