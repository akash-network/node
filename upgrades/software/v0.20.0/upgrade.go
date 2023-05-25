// Package v0_20_0
// nolint revive
package v0_20_0

import (
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	icacontrollertypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/controller/types"
	icahosttypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/types"
	"github.com/tendermint/tendermint/libs/log"

	apptypes "github.com/akash-network/node/app/types"
	utypes "github.com/akash-network/node/upgrades/types"
)

const (
	upgradeName = "v0.20.0"
)

type upgrade struct {
	*apptypes.App
}

var _ utypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(_ log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
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
