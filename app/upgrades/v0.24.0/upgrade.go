// Package v0_24_0
package v0_24_0 // nolint revive

import (
	"fmt"

	"github.com/akash-network/akash-api/go/node/escrow/v1beta3"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/tendermint/tendermint/libs/log"

	agovtypes "github.com/akash-network/akash-api/go/node/gov/v1beta3"
	astakingtypes "github.com/akash-network/akash-api/go/node/staking/v1beta3"

	apptypes "github.com/akash-network/node/app/types"
	agov "github.com/akash-network/node/x/gov"
	astaking "github.com/akash-network/node/x/staking"
)

const (
	UpgradeName = "v0.24.0"
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
		log: log.With(fmt.Sprintf("upgrade/%s", UpgradeName)),
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
		ctx.Logger().Info("initializing parameters in astaking module...")
		if err := up.Keepers.Akash.Staking.SetParams(ctx, astakingtypes.DefaultParams()); err != nil {
			return nil, err
		}

		ctx.Logger().Info("initializing parameters in agov module...")
		if err := up.Keepers.Akash.Gov.SetDepositParams(ctx, agovtypes.DefaultDepositParams()); err != nil {
			return nil, err
		}

		if err := up.enforceMinValidatorCommission(ctx); err != nil {
			return nil, err
		}

		up.patchDanglingEscrowPayments(ctx)

		ctx.Logger().Info("starting module migrations...")
		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}

func (up *upgrade) enforceMinValidatorCommission(ctx sdk.Context) error {
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

func (up *upgrade) patchDanglingEscrowPayments(ctx sdk.Context) {
	up.Keepers.Akash.Escrow.WithPayments(ctx, func(payment v1beta3.FractionalPayment) bool {
		acc, _ := up.Keepers.Akash.Escrow.GetAccount(ctx, payment.AccountID)
		if (payment.State == v1beta3.PaymentOpen && acc.State != v1beta3.AccountOpen) ||
			(payment.State == v1beta3.PaymentOverdrawn && acc.State != v1beta3.AccountOverdrawn) {

			up.log.Info(
				fmt.Sprintf("payment id state `%s:%s` does not match account state `%s:%s`. forcing payment state to %[4]s",
					payment.PaymentID,
					payment.State,
					acc.ID,
					acc.State,
				),
			)

			switch acc.State {
			case v1beta3.AccountOpen:
				payment.State = v1beta3.PaymentOpen
			case v1beta3.AccountClosed:
				payment.State = v1beta3.PaymentClosed
			case v1beta3.AccountOverdrawn:
				payment.State = v1beta3.PaymentOverdrawn
			}
		}

		up.Keepers.Akash.Escrow.SavePayment(ctx, payment)
		return true
	})
}
