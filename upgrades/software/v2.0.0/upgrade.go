// Package v2_0_0
// nolint revive
package v2_0_0

import (
	"context"
	"fmt"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
	epochstypes "pkg.akt.dev/go/node/epochs/v1beta1"
	otypes "pkg.akt.dev/go/node/oracle/v1"
	ttypes "pkg.akt.dev/go/node/take/v1"
	"pkg.akt.dev/go/sdkutil"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
	"pkg.akt.dev/node/v2/x/bme"
	"pkg.akt.dev/node/v2/x/oracle"
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
			epochstypes.StoreKey,
			oracle.StoreKey,
			awasm.StoreKey,
			wasmtypes.StoreKey,
			bme.StoreKey,
		},
		Deleted: []string{
			ttypes.ModuleName,
		},
	}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sctx := sdk.UnwrapSDKContext(ctx)

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
			Name:    "Akash Compute Token",
			Symbol:  "ACT",
		})

		up.Keepers.Cosmos.Bank.SetSendEnabled(ctx, sdkutil.DenomUact, false)

		params := up.Keepers.Cosmos.Wasm.GetParams(ctx)
		// Configure code upload access - RESTRICTED TO GOVERNANCE ONLY
		// Only governance proposals can upload contract code
		// This provides maximum security for mainnet deployment
		params.CodeUploadAccess = wasmtypes.AccessConfig{
			Permission: wasmtypes.AccessTypeNobody,
		}

		// Configure instantiate default permission
		params.InstantiateDefaultPermission = wasmtypes.AccessTypeEverybody

		err = up.Keepers.Cosmos.Wasm.SetParams(ctx, params)
		if err != nil {
			return toVM, err
		}

		dparams := up.Keepers.Akash.Deployment.GetParams(sctx)

		var uakt sdk.Coin
		for _, coin := range dparams.MinDeposits {
			if coin.Denom == sdkutil.DenomUakt {
				uakt = coin
				break
			}
		}

		if uakt.IsNil() {
			panic("uakt coin not found in deployment MinDeposit params")
		}

		dparams.MinDeposits = sdk.Coins{
			sdk.NewInt64Coin(sdkutil.DenomUact, 5000000),
			uakt,
		}

		err = up.Keepers.Akash.Deployment.SetParams(sctx, dparams)
		if err != nil {
			return toVM, err
		}

		oparams := otypes.DefaultParams()

		// Instantiate oracle contracts via wasm message server.
		// The keeper's create method is unexported, so we go through MsgServer.
		// Using the governance module address as a sender grants GovAuthorizationPolicy,
		// which bypasses upload access restrictions.
		pythContractAddr, err := up.instantiateOracleContracts(ctx)
		if err != nil {
			return toVM, fmt.Errorf("failed to instantiate oracle contracts: %w", err)
		}

		// Set the pyth contract as an authorized oracle price source
		oparams.Sources = []string{pythContractAddr}
		err = up.Keepers.Akash.Oracle.SetParams(sctx, oparams)
		if err != nil {
			return toVM, err
		}

		if err := up.migrateDenoms(ctx); err != nil {
			return toVM, fmt.Errorf("failed to migrate denoms: %w", err)
		}

		feePool, err := up.Keepers.Cosmos.Distr.FeePool.Get(ctx)
		if err != nil {
			return toVM, fmt.Errorf("failed to get fee pool: %w", err)
		}

		// deposit 300,000AKT as stated in the upgrade proposal
		bmeDepositAmount := sdk.NewCoin(sdkutil.DenomUakt, sdkmath.NewInt(300000000000))

		err = up.Keepers.Cosmos.Bank.SendCoinsFromModuleToModule(ctx, distrtypes.ModuleName, bmetypes.ModuleName, sdk.Coins{bmeDepositAmount})
		if err != nil {
			return toVM, fmt.Errorf("failed to BME: %w", err)
		}

		feePool.CommunityPool = feePool.CommunityPool.Sub(sdk.DecCoins{sdk.NewDecCoinFromCoin(bmeDepositAmount)})

		err = up.Keepers.Cosmos.Distr.FeePool.Set(ctx, feePool)
		if err != nil {
			return toVM, fmt.Errorf("failed to set updated fee pool balance: %w", err)
		}

		return toVM, err
	}
}
