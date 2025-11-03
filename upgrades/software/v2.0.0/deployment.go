package v2_0_0

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	eid "pkg.akt.dev/go/node/escrow/id/v1"
	emodule "pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/sdkutil"

	utypes "pkg.akt.dev/node/v2/upgrades/types"
	bmemodule "pkg.akt.dev/node/v2/x/bme"
	migrate "pkg.akt.dev/node/v2/x/deployment/migrate/v7"
)

// Known axlUSDC IBC denoms across networks
var denomsAxlUSDC = []string{
	"ibc/170C677610AC31DF0904FFE09CD3B5C657492170E7E52372E48756B71E56F2F1", // mainnet
	"ibc/028CD1864059EEFB48A6048376165318E3E82C234390AE5A6D7B22001725B06E", // sandbox
}

type deploymentMigrations struct {
	utypes.Migrator
}

func newDeploymentMigration(m utypes.Migrator) utypes.Migration {
	return deploymentMigrations{Migrator: m}
}

func (m deploymentMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates deployment from version 6 to 7.
func (m deploymentMigrations) handler(_ sdk.Context) error {
	return nil
}

func isAxlUSDC(denom string) bool {
	for _, d := range denomsAxlUSDC {
		if denom == d {
			return true
		}
	}
	return false
}

func (up *upgrade) migrateDenoms(ctx context.Context) error {
	sctx := sdk.UnwrapSDKContext(ctx)

	dkeeper := up.Keepers.Akash.Deployment
	mkeeper := up.Keepers.Akash.Market
	ekeeper := up.Keepers.Akash.Escrow

	var burnCoin sdk.Coin
	mintCoin := sdk.NewCoin(sdkutil.DenomUact, sdkmath.ZeroInt())

	migration := migrate.NewMigration(dkeeper, mkeeper, ekeeper, up.Keepers.Cosmos.Authz)

	var gerr error

	err := dkeeper.WithDeployments(sctx, func(d dv1.Deployment) bool {
		if d.State != dv1.DeploymentActive {
			return false
		}

		var groups dvbeta.Groups
		groups, gerr = dkeeper.GetGroups(sctx, d.ID)

		if gerr != nil {
			return false
		}

		if len(groups) == 0 {
			return false
		}

		fromDenom := migrate.DetectDenom(groups)

		switch {
		case isAxlUSDC(fromDenom):
			// Immediate migration at 1:1 ratio
			rate := sdkmath.LegacyOneDec()

			srcCoin, dstCoin, err := migration.Run(sctx, d.ID, fromDenom, sdkutil.DenomUact, rate)
			if err != nil {
				up.log.Error("failed to migrate axlUSDC deployment", "deployment", d.ID, "error", err)
				gerr = err
				return true
			}

			if burnCoin.IsNil() {
				burnCoin = sdk.NewCoin(fromDenom, sdkmath.ZeroInt())
			}

			burnCoin = burnCoin.Add(srcCoin)
			mintCoin = mintCoin.Add(dstCoin)
		case fromDenom == sdkutil.DenomUakt:
			// Deferred - store for EndBlocker processing once oracle is available
			if gerr = dkeeper.AddPendingDenomMigration(sctx, d.ID); gerr != nil {
				up.log.Error("failed to add pending denom migration", "deployment", d.ID, "error", gerr)
				return true
			}
		}

		return false
	})
	if err != nil {
		return fmt.Errorf("iterate deployments: %w", err)
	}

	if gerr != nil {
		return fmt.Errorf("migrate deployment: %w", gerr)
	}

	// Second pass: migrate orphaned escrow payments from non-active deployments.
	// Mainnet may have open payments belonging to closed deployments that were
	// not properly cleaned up. The active-deployment loop above skips these.
	ekeeper.WithPayments(sctx, func(p etypes.Payment) bool {
		if p.ID.AID.Scope != eid.ScopeDeployment && (p.State.State != etypes.StateOpen && p.State.State != etypes.StateOverdrawn) {
			return false
		}

		if !isAxlUSDC(p.State.Rate.Denom) {
			return false
		}

		p.State.Rate = sdk.NewDecCoinFromDec(sdkutil.DenomUact, p.State.Rate.Amount)
		p.State.Balance = sdk.NewDecCoinFromDec(sdkutil.DenomUact, p.State.Balance.Amount)
		p.State.Unsettled = sdk.NewDecCoinFromDec(sdkutil.DenomUact, p.State.Unsettled.Amount)
		p.State.Withdrawn = sdk.NewCoin(sdkutil.DenomUact, p.State.Withdrawn.Amount)

		if gerr = ekeeper.SavePaymentRaw(sctx, p); gerr != nil {
			up.log.Error("failed to migrate orphaned payment", "payment", p.ID, "error", gerr)
			return true
		}

		return false
	})

	if gerr != nil {
		return fmt.Errorf("migrate payments: %w", gerr)
	}

	// Third pass: migrate orphaned escrow accounts from non-active deployments.
	ekeeper.WithAccounts(sctx, func(acc etypes.Account) bool {
		if acc.ID.Scope != eid.ScopeDeployment && (acc.State.State != etypes.StateOpen && acc.State.State != etypes.StateOverdrawn) {
			return false
		}

		changed := false

		for i := range acc.State.Funds {
			f := &acc.State.Funds[i]
			origDenom := f.Denom
			if isAxlUSDC(origDenom) {
				srcAmt := f.Amount.TruncateInt()
				f.Denom = sdkutil.DenomUact

				changed = true

				if srcAmt.GT(sdkmath.ZeroInt()) {
					if burnCoin.IsNil() {
						burnCoin = sdk.NewCoin(origDenom, sdkmath.ZeroInt())
					}

					burnCoin = burnCoin.Add(sdk.NewCoin(origDenom, srcAmt))
					mintCoin = mintCoin.Add(sdk.NewCoin(sdkutil.DenomUact, srcAmt))
				}
			}
		}

		for i := range acc.State.Deposits {
			d := &acc.State.Deposits[i]
			if isAxlUSDC(d.Balance.Denom) {
				d.Balance.Denom = sdkutil.DenomUact
				changed = true
			}
		}

		acc.State.Transferred = append(acc.State.Transferred, sdk.NewDecCoin(sdkutil.DenomUact, sdkmath.ZeroInt()))

		if changed {
			if gerr = ekeeper.SaveAccountRaw(sctx, acc); gerr != nil {
				up.log.Error("failed to migrate orphaned escrow account", "account", acc.ID, "error", gerr)
				return true
			}
		}

		return false
	})

	if gerr != nil {
		return fmt.Errorf("migrate escrow accounts: %w", gerr)
	}

	if mintCoin.IsGT(sdk.NewCoin(sdkutil.DenomUact, sdkmath.ZeroInt())) {
		err = up.Keepers.Cosmos.Bank.SendCoinsFromModuleToModule(ctx, emodule.ModuleName, bmemodule.ModuleName, sdk.Coins{burnCoin})
		if err != nil {
			return nil
		}

		err = up.Keepers.Cosmos.Bank.BurnCoins(ctx, bmemodule.ModuleName, sdk.Coins{burnCoin})
		if err != nil {
			return nil
		}

		err = up.Keepers.Cosmos.Bank.MintCoins(ctx, bmemodule.ModuleName, sdk.Coins{mintCoin})
		if err != nil {
			return nil
		}

		err = up.Keepers.Cosmos.Bank.SendCoinsFromModuleToModule(ctx, bmemodule.ModuleName, emodule.ModuleName, sdk.Coins{mintCoin})
		if err != nil {
			return nil
		}
	}

	return nil
}
