// Package v0_22_0
package v0_22_0 // nolint revive

import (
	"fmt"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/tendermint/tendermint/libs/log"

	apptypes "github.com/akash-network/node/app/types"
	agov "github.com/akash-network/node/x/gov"
	astaking "github.com/akash-network/node/x/staking"
)

const (
	UpgradeName = "v0.22.0"
)

func init() {
	apptypes.RegisterUpgrade(UpgradeName, initUpgrade)
}

type upgrade struct {
	*apptypes.App
	log log.Logger
}

var _ apptypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(log log.Logger, app *apptypes.App) (apptypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
		log: log.With("upgrade/v0.22.0"),
	}

	if _, exists := up.MM.Modules[agov.ModuleName]; !exists {
		return nil, fmt.Errorf("module %s has not been initialized", agov.ModuleName) // nolint: goerr113
	}

	if _, exists := up.MM.Modules[astaking.ModuleName]; !exists {
		return nil, fmt.Errorf("module %s has not been initialized", astaking.ModuleName) // nolint: goerr113
	}

	return up, nil
}

func (up *upgrade) StoreLoader() *storetypes.StoreUpgrades {
	upgrades := &storetypes.StoreUpgrades{
		Added: []string{
			feegrant.StoreKey,
			agov.StoreKey,
			astaking.StoreKey,
		},
	}

	return upgrades
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		ctx.Logger().Info("start to run module migrations...")
		fromVM[feegrant.ModuleName] = up.MM.Modules[feegrant.ModuleName].ConsensusVersion()
		fromVM[astaking.ModuleName] = up.MM.Modules[astaking.ModuleName].ConsensusVersion()
		fromVM[agov.ModuleName] = up.MM.Modules[agov.ModuleName].ConsensusVersion()

		if err := up.patchValidatorsCommission(ctx); err != nil {
			return nil, err
		}

		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}

func (up *upgrade) patchValidatorsCommission(ctx sdk.Context) error {
	minRate := up.Keepers.Akash.Staking.MinCommissionRate(ctx)

	validators := up.Keepers.Cosmos.Staking.GetAllValidators(ctx)

	for _, validator := range validators {
		// update MaxRate if it is less than minimum required rate
		if validator.Commission.MaxRate.LT(minRate) {
			validator.Commission.MaxRate = minRate
		}

		if validator.GetCommission().LT(minRate) {
			up.log.Info(
				fmt.Sprintf("validator's `%s` current commission is %s%% < %[3]s%%(min required). Force updating to %[3]s%%",
					validator.OperatorAddress,
					validator.Commission.Rate,
					minRate),
			)
			// set max change rate temporarily to 100%
			maxRateCh := validator.Commission.MaxChangeRate
			validator.Commission.MaxChangeRate = sdk.NewDecWithPrec(1, 0)
			if _, err := up.Keepers.Cosmos.Staking.UpdateValidatorCommission(ctx, validator, minRate); err != nil {
				return err
			}

			validator.Commission.MaxChangeRate = maxRateCh

			up.Keepers.Cosmos.Staking.BeforeValidatorModified(ctx, validator.GetOperator())
			up.Keepers.Cosmos.Staking.SetValidator(ctx, validator)
		}
	}

	return nil
}
