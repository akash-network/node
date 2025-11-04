// Package v2_0_0
// nolint revive
package v2_0_0

import (
	"context"
	"fmt"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/CosmWasm/wasmd/x/wasm"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
	awasm "pkg.akt.dev/node/v2/x/wasm"
)

const (
	UpgradeName = "v2.0.0"
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
			awasm.ModuleName,
			// With the migrations of all modules away from x/params, the crisis module now has a store.
			// The store must be created during a chain upgrade to v0.53.x.
			wasmtypes.ModuleName,
		},
		Deleted: []string{},
	}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// Set wasm old version to 1 if we want to call wasm's InitGenesis ourselves
		// in this upgrade logic ourselves.
		//
		// vm[wasm.ModuleName] = wasm.ConsensusVersion
		//
		// Otherwise we run this, which will run wasm.InitGenesis(wasm.DefaultGenesis())
		// and then override it after.

		// Set the initial wasm module version
		fromVM[wasmtypes.ModuleName] = wasm.AppModule{}.ConsensusVersion()

		// Set default wasm params
		params := wasmtypes.DefaultParams()

		// Configure code upload access - RESTRICTED TO GOVERNANCE ONLY
		// Only governance proposals can upload contract code
		// This provides maximum security for mainnet deployment
		params.CodeUploadAccess = wasmtypes.AccessConfig{
			Permission: wasmtypes.AccessTypeNobody,
		}

		params.CodeUploadAccess = wasmtypes.AllowNobody
		// Configure instantiate default permission
		params.InstantiateDefaultPermission = wasmtypes.AccessTypeEverybody

		err := up.Keepers.External.Wasm.SetParams(ctx, params)
		if err != nil {
			return fromVM, err
		}

		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}
