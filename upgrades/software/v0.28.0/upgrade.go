// Package v0_28_0
// nolint revive
package v0_28_0

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/authz"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	apptypes "github.com/akash-network/node/app/types"
	utypes "github.com/akash-network/node/upgrades/types"
)

const (
	UpgradeName = "v0.28.0"
)

type upgrade struct {
	*apptypes.App
	log log.Logger
}

var _ utypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(log log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
		log: log.With("module", fmt.Sprintf("upgrade/%s", UpgradeName)),
	}

	return up, nil
}

func (up *upgrade) StoreLoader() *storetypes.StoreUpgrades {
	return &storetypes.StoreUpgrades{}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}

type grantBackup struct {
	granter sdk.AccAddress
	grantee sdk.AccAddress
	grant   authz.Grant
}
