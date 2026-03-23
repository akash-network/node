package keeper

import (
	"context"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v1 "pkg.akt.dev/go/node/deployment/v1"
	emodule "pkg.akt.dev/go/node/escrow/module"
	"pkg.akt.dev/go/sdkutil"

	bmemodule "pkg.akt.dev/node/v2/x/bme"
	"pkg.akt.dev/node/v2/x/deployment/keeper/keys"
	migrate "pkg.akt.dev/node/v2/x/deployment/migrate/v7"
)

func (k Keeper) EndBlocker(ctx context.Context) error {
	sctx := sdk.UnwrapSDKContext(ctx)

	iter, err := k.pendingDenomMigrations.Iterate(ctx, nil)
	if err != nil {
		sctx.Logger().Error(err.Error())
		return nil
	}
	defer func() {
		_ = iter.Close()
	}()

	if !iter.Valid() {
		// all pending migrations completed
		return nil
	}

	rate, err := k.oracleKeeper.GetAggregatedPrice(sctx, sdkutil.DenomUakt)
	if err != nil {
		sctx.Logger().Error(err.Error())
		return nil
	}

	migration := migrate.NewMigration(k, k.marketKeeper, k.ekeeper, k.authzKeeper)

	burnCoin := sdk.NewCoin(sdkutil.DenomUakt, sdkmath.ZeroInt())
	mintCoin := sdk.NewCoin(sdkutil.DenomUact, sdkmath.ZeroInt())

	var migrated []v1.DeploymentID

	count := 0
	const maxPerBlock = 50
	err = k.pendingDenomMigrations.Walk(ctx, nil, func(pk keys.DeploymentPrimaryKey, _ sdkmath.Int) (bool, error) {
		if count >= maxPerBlock {
			return true, nil // stop
		}

		did := keys.KeyToDeploymentID(pk)

		srcCoin, dstCoin, err := migration.Run(sctx, did, sdkutil.DenomUakt, sdkutil.DenomUact, rate)
		if err != nil {
			return true, err
		}

		if srcCoin.Denom != burnCoin.Denom {
			return false, nil
		}

		burnCoin = burnCoin.Add(srcCoin)
		mintCoin = mintCoin.Add(dstCoin)

		migrated = append(migrated, did)
		return false, nil
	})

	if mintCoin.IsGT(sdk.NewCoin(sdkutil.DenomUact, sdkmath.ZeroInt())) {
		err = k.bankKeeper.SendCoinsFromModuleToModule(ctx, emodule.ModuleName, bmemodule.ModuleName, sdk.Coins{burnCoin})
		if err != nil {
			return err
		}

		err = k.bankKeeper.BurnCoins(ctx, bmemodule.ModuleName, sdk.Coins{burnCoin})
		if err != nil {
			return err
		}

		err = k.bankKeeper.MintCoins(ctx, bmemodule.ModuleName, sdk.Coins{mintCoin})
		if err != nil {
			return err
		}

		err = k.bankKeeper.SendCoinsFromModuleToModule(ctx, bmemodule.ModuleName, emodule.ModuleName, sdk.Coins{mintCoin})
		if err != nil {
			return err
		}
	}

	for _, did := range migrated {
		err = k.pendingDenomMigrations.Remove(ctx, keys.DeploymentIDToKey(did))
		if err != nil {
			sctx.Logger().Error("failed to remove migration for deployment %s: %v", did, err)
		}
	}

	return nil
}
