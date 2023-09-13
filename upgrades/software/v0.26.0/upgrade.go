// Package v0_26_0
// nolint revive
package v0_26_0

import (
	"fmt"
	"reflect"
	"time"

	"github.com/tendermint/tendermint/libs/log"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	v1beta2dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta2"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	apptypes "github.com/akash-network/node/app/types"
	utypes "github.com/akash-network/node/upgrades/types"
)

const (
	UpgradeName = "v0.26.0"
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
	return &storetypes.StoreUpgrades{}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		if err := up.enforceMinValidatorCommission(ctx); err != nil {
			return nil, err
		}

		if err := up.migrateDeploymentAuthz(ctx); err != nil {
			return nil, err
		}

		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}

type grantBackup struct {
	granter sdk.AccAddress
	grantee sdk.AccAddress
	grant   authz.Grant
}

func (up *upgrade) migrateDeploymentAuthz(ctx sdk.Context) error {
	msgUrlOld := v1beta2dtypes.DepositDeploymentAuthorization{}.MsgTypeURL()

	grants := make([]grantBackup, 0, 10000)

	up.Keepers.Cosmos.Authz.IterateGrants(ctx, func(granterAddr sdk.AccAddress, granteeAddr sdk.AccAddress, grant authz.Grant) bool {
		authorization := grant.GetAuthorization()
		if authorization.MsgTypeURL() == msgUrlOld {
			grants = append(grants, grantBackup{
				granter: granterAddr,
				grantee: granteeAddr,
				grant:   grant,
			})
		}
		return false
	})

	expiredCnt := 0

	for _, grant := range grants {
		authzGen := grant.grant.GetAuthorization()
		authzOld, valid := authzGen.(*v1beta2dtypes.DepositDeploymentAuthorization)
		if !valid {
			return fmt.Errorf("unexpected authorization type. expected (%s), actual (%s)", reflect.TypeOf(&v1beta2dtypes.DepositDeploymentAuthorization{}), reflect.TypeOf(authzGen))
		}

		err := up.Keepers.Cosmos.Authz.DeleteGrant(ctx, grant.grantee, grant.granter, msgUrlOld)
		if err != nil {
			return err
		}

		// Save only grants that are still active at upgrade block
		if grant.grant.Expiration.Before(ctx.BlockHeader().Time) {
			expiredCnt++
			continue
		}

		authzNew := dtypes.NewDepositDeploymentAuthorization(authzOld.SpendLimit)

		err = up.Keepers.Cosmos.Authz.SaveGrant(ctx, grant.grantee, grant.granter, authzNew, grant.grant.Expiration)
		if err != nil {
			return err
		}
	}

	up.log.Info("updated DepositDeploymentAuthorizations from v1beta2 to v1beta3", "total", len(grants), "expired", expiredCnt)

	return nil
}

func (up *upgrade) enforceMinValidatorCommission(ctx sdk.Context) error {
	minRate := up.Keepers.Akash.Staking.MinCommissionRate(ctx)
	validators := up.Keepers.Cosmos.Staking.GetAllValidators(ctx)

	for _, validator := range validators {
		if validator.Commission.MaxRate.LT(minRate) || validator.GetCommission().LT(minRate) {
			// update MaxRate if it is less than minimum required rate
			if validator.Commission.MaxRate.LT(minRate) {
				up.log.Info(fmt.Sprintf("force updating validator commission MaxRate to %s", minRate),
					"address",
					validator.OperatorAddress,
					"currentMaxRate",
					validator.Commission.MaxRate,
				)

				validator.Commission.MaxRate = minRate
			}

			if validator.GetCommission().LT(minRate) {
				up.log.Info(fmt.Sprintf("force updating validator commission Rate to %s", minRate),
					"address",
					validator.OperatorAddress,
					"currentRate",
					validator.Commission.Rate,
				)

				// set max change rate temporarily to 100%
				maxRateCh := validator.Commission.MaxChangeRate
				validator.Commission.MaxChangeRate = sdk.NewDecWithPrec(1, 0)

				newCommission, err := updateValidatorCommission(ctx, validator, minRate)
				if err != nil {
					return err
				}

				validator.Commission = newCommission
				validator.Commission.MaxChangeRate = maxRateCh
			}

			up.Keepers.Cosmos.Staking.BeforeValidatorModified(ctx, validator.GetOperator())
			up.Keepers.Cosmos.Staking.SetValidator(ctx, validator)
		}
	}

	return nil
}

// updateValidatorCommission use custom implementation of update commission,
// this prevents panic during upgrade if any of validators have changed their
// commission within 24h of upgrade height
func updateValidatorCommission(
	ctx sdk.Context,
	validator stakingtypes.Validator,
	newRate sdk.Dec,
) (stakingtypes.Commission, error) {
	commission := validator.Commission
	blockTime := ctx.BlockHeader().Time

	if err := validateNewRate(commission, newRate, blockTime); err != nil {
		return commission, err
	}

	commission.Rate = newRate
	commission.UpdateTime = blockTime

	return commission, nil
}

// validateNewRate performs basic sanity validation checks of a new commission
// rate. If validation fails, an SDK error is returned.
func validateNewRate(commission stakingtypes.Commission, newRate sdk.Dec, _ time.Time) error {
	switch {
	case newRate.IsNegative():
		// new rate cannot be negative
		return stakingtypes.ErrCommissionNegative

	case newRate.GT(commission.MaxRate):
		// new rate cannot be greater than the max rate
		return stakingtypes.ErrCommissionGTMaxRate

	case newRate.Sub(commission.Rate).GT(commission.MaxChangeRate):
		// new rate % points change cannot be greater than the max change rate
		return stakingtypes.ErrCommissionGTMaxChangeRate
	}

	return nil
}
