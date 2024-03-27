// Package v0_33_0
// nolint revive
package v0_33_0

import (
	"fmt"
	"github.com/tendermint/tendermint/libs/log"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	icahosttypes "github.com/cosmos/ibc-go/v4/modules/apps/27-interchain-accounts/host/types"

	apptypes "github.com/akash-network/node/app/types"
	utypes "github.com/akash-network/node/upgrades/types"
)

const (
	UpgradeName = "v0.33.0"
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
	return &storetypes.StoreUpgrades{
		Added: []string{icahosttypes.StoreKey},
	}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// let migrations run so that new stores are added
		newVM, err := up.MM.RunMigrations(ctx, up.Configurator, fromVM)
		if err != nil {
			return nil, err
		}
		// edit the stores
		icaHostParams := up.Keepers.Cosmos.ICAHostKeeper.GetParams(ctx)
		icaHostParams.HostEnabled = true
		icaHostParams.AllowMessages = []string{
			"/cosmos.bank.v1beta1.MsgSend",
			"/cosmos.staking.v1beta1.MsgDelegate",
			"/cosmos.staking.v1beta1.MsgUndelegate",
			"/cosmos.staking.v1beta1.MsgBeginRedelegate",
			"/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward",
			"/cosmos.distribution.v1beta1.MsgSetWithdrawAddress",
			"/ibc.applications.transfer.v1.MsgTransfer",
		}
		up.Keepers.Cosmos.ICAHostKeeper.SetParams(ctx, icaHostParams)
		return newVM, err
	}
}
