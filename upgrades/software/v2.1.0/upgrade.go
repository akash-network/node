// Package v2_1_0
// nolint revive
package v2_1_0

import (
	"context"
	"fmt"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	etypes "pkg.akt.dev/go/node/escrow/module"
	otypes "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
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
		Added:   []string{},
		Deleted: []string{},
	}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sctx := sdk.UnwrapSDKContext(ctx)

		toVM, err := up.MM.RunMigrations(ctx, up.Configurator, fromVM)
		if err != nil {
			return toVM, err
		}

		msgServer := wasmkeeper.NewMsgServerImpl(up.Keepers.Cosmos.Wasm)
		govAddr := up.Keepers.Cosmos.Wasm.GetAuthority()

		contractAddr := "akash1nc5tatafv6eyq7llkr2gv50ff9e22mnf70qgjlv737ktmt4eswrqyagled"

		_, err = msgServer.StoreAndMigrateContract(ctx, &wasmtypes.MsgStoreAndMigrateContract{
			Authority:             govAddr,
			WASMByteCode:          pythContract,
			Contract:              contractAddr,
			InstantiatePermission: &wasmtypes.AllowNobody,
			Msg:                   []byte("{}"),
		})
		if err != nil {
			return toVM, err
		}

		oparams := otypes.DefaultParams()
		oparams.MinPriceSources = 1
		// Set the pyth contract as an authorized oracle price source
		oparams.Sources = []string{contractAddr}
		err = up.Keepers.Akash.Oracle.SetParams(sctx, oparams)
		if err != nil {
			return toVM, err
		}

		if sctx.ChainID() == "akashnet-2" {
			feePool, err := up.Keepers.Cosmos.Distr.FeePool.Get(ctx)
			if err != nil {
				return toVM, fmt.Errorf("failed to get fee pool: %w", err)
			}

			// deposit 427,414,453uAKT to escrow as stated in the upgrade proposal
			escrowDepositAmount := sdk.NewCoin(sdkutil.DenomUakt, sdkmath.NewInt(427414453))

			err = up.Keepers.Cosmos.Bank.SendCoinsFromModuleToModule(ctx, distrtypes.ModuleName, etypes.ModuleName, sdk.Coins{escrowDepositAmount})
			if err != nil {
				return toVM, fmt.Errorf("failed to transfer funds to escrow: %w", err)
			}

			feePool.CommunityPool = feePool.CommunityPool.Sub(sdk.DecCoins{sdk.NewDecCoinFromCoin(escrowDepositAmount)})

			err = up.Keepers.Cosmos.Distr.FeePool.Set(ctx, feePool)
			if err != nil {
				return toVM, fmt.Errorf("failed to set updated fee pool balance: %w", err)
			}
		}

		bparams, err := up.Keepers.Akash.Bme.GetParams(sctx)
		if err != nil {
			return toVM, fmt.Errorf("failed to get bme params: %w", err)
		}

		if bparams.MaxPendingAttempts == 0 {
			bparams.MaxPendingAttempts = 3
			err = up.Keepers.Akash.Bme.SetParams(sctx, bparams)
			if err != nil {
				return toVM, fmt.Errorf("failed to set bme params: %w", err)
			}
		}

		return toVM, err
	}
}
