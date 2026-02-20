// Package v2_1_0
// nolint revive
package v2_1_0

import (
	"context"
	"fmt"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ttypes "pkg.akt.dev/go/node/take/v1"
	"pkg.akt.dev/go/sdkutil"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
	"pkg.akt.dev/node/v2/x/bme"
)

const (
	UpgradeName = "v2.1.0"
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
		Added: []string{
			bme.StoreKey,
		},
		Deleted: []string{
			ttypes.ModuleName,
		},
	}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		toVM, err := up.MM.RunMigrations(ctx, up.Configurator, fromVM)
		if err != nil {
			return toVM, err
		}

		up.Keepers.Cosmos.Bank.SetDenomMetaData(ctx, banktypes.Metadata{
			Description: "Akash Compute Token",
			DenomUnits: []*banktypes.DenomUnit{
				{
					Denom:    sdkutil.DenomAct,
					Exponent: 6,
				},
				{
					Denom:    sdkutil.DenomMact,
					Exponent: 3,
				},
				{
					Denom:    sdkutil.DenomUact,
					Exponent: 0,
				},
			},
			Base:    sdkutil.DenomUact,
			Display: sdkutil.DenomUact,
			Name:    sdkutil.DenomUact,
			Symbol:  sdkutil.DenomUact,
			URI:     "",
			URIHash: "",
		})

		up.Keepers.Cosmos.Bank.SetSendEnabled(ctx, sdkutil.DenomUact, false)

		return toVM, err
	}
}
