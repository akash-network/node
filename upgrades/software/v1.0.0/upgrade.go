// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	dv1beta3 "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	agovtypes "pkg.akt.dev/go/node/gov/v1beta3"
	astakingtypes "pkg.akt.dev/go/node/staking/v1beta3"
	taketypes "pkg.akt.dev/go/node/take/v1"

	apptypes "pkg.akt.dev/node/app/types"
	utypes "pkg.akt.dev/node/upgrades/types"
)

const (
	UpgradeName = "v1.0.0"
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
			// With the migrations of all modules away from x/params, the crisis module now has a store.
			// The store must be created during a chain upgrade to v0.47.x.

			// Because the x/consensus module is a new module, its store must be added while upgrading to v0.47.x:
			consensustypes.ModuleName,
		},
		Deleted: []string{
			"agov",
			"astaking",
			crisistypes.ModuleName,
		},
	}
}

type AccountKeeper interface {
	NewAccount(sdk.Context, sdk.AccountI) sdk.AccountI

	GetAccount(ctx sdk.Context, addr sdk.AccAddress) sdk.AccountI
	SetAccount(ctx sdk.Context, acc sdk.AccountI)
}

// AkashUtilsExtraAccountTypes is a map of extra account types that can be overridden.
// This is defined as a global variable, so it can be modified in the chain's app.go and used here without
// having to import the chain. Specifically, this is used for compatibility with Akash' Cosmos SDK fork
var AkashUtilsExtraAccountTypes map[reflect.Type]struct{}

// CanCreateModuleAccountAtAddr tells us if we can safely make a module account at
// a given address. By collision resistance of the address (given API safe construction),
// the only way for an account to be already be at this address is if its claimed by the same
// pre-image from the correct module,
// or some SDK command breaks assumptions and creates an account at designated address.
// This function checks if there is an account at that address, and runs some safety checks
// to be extra-sure its not a user account (e.g. non-zero sequence, pubkey, of fore-seen account types).
// If there is no account, or if we believe its not a user-spendable account, we allow module account
// creation at the address.
// else, we do not.
//
// TODO: This is generally from an SDK design flaw
// code based off wasmd code: https://github.com/CosmWasm/wasmd/pull/996
// Its _mandatory_ that the caller do the API safe construction to generate a module account addr,
// namely, address.Module(ModuleName, {key})
func CanCreateModuleAccountAtAddr(ctx sdk.Context, ak AccountKeeper, addr sdk.AccAddress) error {
	existingAcct := ak.GetAccount(ctx, addr)
	if existingAcct == nil {
		return nil
	}
	if existingAcct.GetSequence() != 0 || existingAcct.GetPubKey() != nil {
		return fmt.Errorf("cannot create module account %s, "+
			"due to an account at that address already existing & having sent txs", addr)
	}
	overrideAccountTypes := map[reflect.Type]struct{}{
		reflect.TypeOf(&authtypes.BaseAccount{}):                 {},
		reflect.TypeOf(&vestingtypes.DelayedVestingAccount{}):    {},
		reflect.TypeOf(&vestingtypes.ContinuousVestingAccount{}): {},
		reflect.TypeOf(&vestingtypes.BaseVestingAccount{}):       {},
		reflect.TypeOf(&vestingtypes.PeriodicVestingAccount{}):   {},
		reflect.TypeOf(&vestingtypes.PermanentLockedAccount{}):   {},
	}
	for extraAccountType := range AkashUtilsExtraAccountTypes {
		overrideAccountTypes[extraAccountType] = struct{}{}
	}

	if _, clear := overrideAccountTypes[reflect.TypeOf(existingAcct)]; clear {
		return nil
	}

	return errors.New("cannot create module account %s, " +
		"due to an account at that address already existing & not being an overridable type")
}

// CreateModuleAccountByName creates a module account at the provided name
func CreateModuleAccountByName(ctx sdk.Context, ak AccountKeeper, name string) error {
	addr := authtypes.NewModuleAddress(name)
	err := CanCreateModuleAccountAtAddr(ctx, ak, addr)
	if err != nil {
		return err
	}

	acc := ak.NewAccount(
		ctx,
		authtypes.NewModuleAccount(
			authtypes.NewBaseAccountWithAddress(addr),
			name,
		),
	)
	ak.SetAccount(ctx, acc)
	return nil
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	baseAppLegacySS := up.Keepers.Cosmos.Params.Subspace(baseapp.Paramspace).WithKeyTable(paramstypes.ConsensusParamsKeyTable())

	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// Migrate Tendermint consensus parameters from x/params module to a
		// dedicated x/consensus module.
		sctx := sdk.UnwrapSDKContext(ctx)

		err := baseapp.MigrateParams(sctx, baseAppLegacySS, up.Keepers.Cosmos.ConsensusParams.ParamsStore)
		if err != nil {
			return nil, err
		}
		sspace, exists := up.Keepers.Cosmos.Params.GetSubspace(stakingtypes.ModuleName)
		if !exists {
			return nil, fmt.Errorf("params subspace \"%s\" not found", stakingtypes.ModuleName)
		}

		up.log.Info("migrating take params to store")
		sspace, exists = up.Keepers.Cosmos.Params.GetSubspace(taketypes.ModuleName)
		if !exists {
			return nil, fmt.Errorf("params subspace \"%s\" not found", taketypes.ModuleName)
		}

		tparams := taketypes.Params{}
		sspace.Get(sctx, taketypes.KeyDefaultTakeRate, &tparams.DefaultTakeRate)
		sspace.Get(sctx, taketypes.KeyDenomTakeRates, &tparams.DenomTakeRates)

		err = up.Keepers.Akash.Take.SetParams(sctx, tparams)
		if err != nil {
			return nil, err
		}

		up.log.Info(fmt.Sprintf("migrating param agov.MinInitialDepositRate to gov.MinInitialDepositRatio"))
		sspace, exists = up.Keepers.Cosmos.Params.GetSubspace("agov")
		if !exists {
			return nil, fmt.Errorf("params subspace \"%s\" not found", "agov")
		}

		dparams := agovtypes.DepositParams{}
		sspace.Get(sctx, agovtypes.KeyDepositParams, &dparams)

		toVM, err := up.MM.RunMigrations(ctx, up.Configurator, fromVM)
		if err != nil {
			return nil, err
		}

		// patch deposit authorizations after authz store upgrade
		err = up.patchDepositAuthorizations(sctx)
		if err != nil {
			return nil, err
		}

		// Migrate governance min deposit parameter to builtin gov params
		gparams, err := up.Keepers.Cosmos.Gov.Params.Get(ctx)
		if err != nil {
			return nil, err
		}

		gparams.MinInitialDepositRatio = dparams.MinInitialDepositRate.String()

		// min deposit for an expedited proposal is set to 2000AKT
		gparams.ExpeditedMinDeposit = sdk.NewCoins(sdk.NewCoin("uakt", sdkmath.NewInt(2000000000)))
		gparams.ExpeditedThreshold = sdkmath.LegacyNewDecWithPrec(667, 3).String()

		eVotePeriod := time.Hour * 24
		gparams.ExpeditedVotingPeriod = &eVotePeriod

		err = up.Keepers.Cosmos.Gov.Params.Set(ctx, gparams)
		if err != nil {
			return nil, err
		}

		up.log.Info(fmt.Sprintf("migrating param astaking.MinCommissionRate to staking.MinCommissionRate"))
		sspace, exists = up.Keepers.Cosmos.Params.GetSubspace(astakingtypes.ModuleName)
		if !exists {
			return nil, fmt.Errorf("params subspace \"%s\" not found", astakingtypes.ModuleName)
		}

		sparam := sdkmath.LegacyDec{}
		sspace.Get(sctx, astakingtypes.KeyMinCommissionRate, &sparam)

		sparams, err := up.Keepers.Cosmos.Staking.GetParams(sctx)
		if err != nil {
			return nil, err
		}
		sparams.MinCommissionRate = sparam

		err = up.Keepers.Cosmos.Staking.SetParams(ctx, sparams)
		if err != nil {
			return nil, err
		}

		return toVM, err
	}
}

type grantBackup struct {
	murl       string
	granter    sdk.AccAddress
	grantee    sdk.AccAddress
	expiration *time.Time
	auth       *dv1beta3.DepositDeploymentAuthorization
}

func (up *upgrade) patchDepositAuthorizations(ctx sdk.Context) error {
	//msgUrlOld := "/akash.deployment.v1beta3.MsgDepositDeployment"
	//
	//grants := make([]grantBackup, 0, 10000)

	expiredGrants := 0

	up.Keepers.Cosmos.Authz.IterateGrants(ctx, func(granterAddr sdk.AccAddress, granteeAddr sdk.AccAddress, grant authz.Grant) bool {
		//authorization, err := grant.GetAuthorization()
		//if err != nil {
		//	up.log.Error(fmt.Sprintf("unable to get autorization. err=%s", err.Error()))
		//	return false
		//}
		//
		//if grant.Expiration.Before(ctx.BlockHeader().Time) {
		//	expiredGrants++
		//	return false
		//}
		//
		//authzOld, valid := authorization.(*dv1beta3.DepositDeploymentAuthorization)
		//if !valid {
		//	return false
		//}
		//
		//grants = append(grants, grantBackup{
		//	murl:       msgUrlOld,
		//	granter:    granterAddr,
		//	grantee:    granteeAddr,
		//	expiration: grant.Expiration,
		//	auth:       authzOld,
		//})

		return false
	})

	//err := up.Keepers.Cosmos.Authz.DequeueAndDeleteExpiredGrants(ctx, expiredGrants)
	//if err != nil {
	//	return err
	//}
	//
	//for _, grant := range grants {
	//	err := up.Keepers.Cosmos.Authz.DeleteGrant(ctx, grant.grantee, grant.granter, grant.murl)
	//	if err != nil {
	//		up.log.Error(fmt.Sprintf("unable to delete autorization. err=%s", err.Error()))
	//	}
	//
	//	authzNew := dv1.NewDepositAuthorization(grant.auth.SpendLimit)
	//	err = up.Keepers.Cosmos.Authz.SaveGrant(ctx, grant.grantee, grant.granter, authzNew, grant.expiration)
	//	if err != nil {
	//		return err
	//	}
	//}

	up.log.Info(fmt.Sprintf("cleaned %d expired grants", expiredGrants))

	return nil
}
