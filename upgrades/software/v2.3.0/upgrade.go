package v2_3_0

import (
	"context"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	apptypes "pkg.akt.dev/node/v2/app/types"
)

const UpgradeName = "v2.3.0"

type upgrade struct {
	*apptypes.App
	log log.Logger
}

func (*upgrade) StoreLoader() *storetypes.StoreUpgrades {
	return &storetypes.StoreUpgrades{}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, p upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sctx := sdk.UnwrapSDKContext(ctx)

		toVM, err := up.MM.RunMigrations(sctx, up.Configurator, fromVM)
		if err != nil {
			return toVM, err
		}

		return toVM, nil
	}
}
