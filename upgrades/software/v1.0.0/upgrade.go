// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/cosmos/cosmos-sdk/baseapp"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	auctiontypes "github.com/skip-mev/block-sdk/x/auction/types"

	dv1beta3 "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	agovtypes "pkg.akt.dev/go/node/gov/v1beta3"
	astakingtypes "pkg.akt.dev/go/node/staking/v1beta3"
	taketypes "pkg.akt.dev/go/node/take/v1"

	apptypes "pkg.akt.dev/akashd/app/types"
	utypes "pkg.akt.dev/akashd/upgrades/types"
	agov "pkg.akt.dev/akashd/x/gov"
	astaking "pkg.akt.dev/akashd/x/staking"
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
			crisistypes.ModuleName,
			// Because the x/consensus module is a new module, its store must be added while upgrading to v0.47.x:
			consensustypes.ModuleName,
			auctiontypes.ModuleName,
		},
		Deleted: []string{
			agov.ModuleName,
			astaking.ModuleName,
		},
	}
}

type AccountKeeper interface {
	NewAccount(sdk.Context, authtypes.AccountI) authtypes.AccountI

	GetAccount(ctx sdk.Context, addr sdk.AccAddress) authtypes.AccountI
	SetAccount(ctx sdk.Context, acc authtypes.AccountI)
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

// const (
// 	// Noble USDC is used as the auction denom
// 	AuctionUSDCDenom = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
// )
//
// // AuctionParams expected initial params for the block-sdk
// var AuctionParams = auctiontypes.Params{
// 	MaxBundleSize:          5,
// 	ReserveFee:             sdk.NewCoin(AuctionUSDCDenom, sdk.NewInt(1000000)),
// 	MinBidIncrement:        sdk.NewCoin(AuctionUSDCDenom, sdk.NewInt(1000000)),
// 	EscrowAccountAddress:   auctiontypes.DefaultEscrowAccountAddress,
// 	FrontRunningProtection: true,
// 	ProposerFee:            math.LegacyMustNewDecFromStr("0.05"),
// }

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

	return func(ctx sdk.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// Migrate Tendermint consensus parameters from x/params module to a
		// dedicated x/consensus module.
		baseapp.MigrateParams(ctx, baseAppLegacySS, up.Keepers.Cosmos.ConsensusParams)

		sspace, exists := up.Keepers.Cosmos.Params.GetSubspace(stakingtypes.ModuleName)
		if !exists {
			return nil, fmt.Errorf("params subspace \"%s\" not found", stakingtypes.ModuleName)
		}
		nilDec := sdk.Dec{}

		// during testnetify following parameters are missing in the exported state
		// so give them default values if missing
		sspace.GetIfExists(ctx, stakingtypes.KeyValidatorBondFactor, &nilDec)
		if nilDec.IsNil() {
			sspace.Set(ctx, stakingtypes.KeyValidatorBondFactor, &stakingtypes.DefaultValidatorBondFactor)
		}

		nilDec = sdk.Dec{}
		sspace.GetIfExists(ctx, stakingtypes.KeyGlobalLiquidStakingCap, &nilDec)
		if nilDec.IsNil() {
			sspace.Set(ctx, stakingtypes.KeyGlobalLiquidStakingCap, &stakingtypes.DefaultGlobalLiquidStakingCap)
		}

		nilDec = sdk.Dec{}
		sspace.GetIfExists(ctx, stakingtypes.KeyValidatorLiquidStakingCap, &nilDec)
		if nilDec.IsNil() {
			sspace.Set(ctx, stakingtypes.KeyValidatorLiquidStakingCap, &stakingtypes.DefaultValidatorLiquidStakingCap)
		}

		up.log.Info("migrating take params to store")
		sspace, exists = up.Keepers.Cosmos.Params.GetSubspace(taketypes.ModuleName)
		if !exists {
			return nil, fmt.Errorf("params subspace \"%s\" not found", taketypes.ModuleName)
		}

		tparams := taketypes.Params{}
		sspace.Get(ctx, taketypes.KeyDefaultTakeRate, &tparams.DefaultTakeRate)
		sspace.Get(ctx, taketypes.KeyDenomTakeRates, &tparams.DenomTakeRates)

		err := up.Keepers.Akash.Take.SetParams(ctx, tparams)
		if err != nil {
			return nil, err
		}

		up.log.Info(fmt.Sprintf("migrating param agov.MinInitialDepositRate to gov.MinInitialDepositRatio"))
		sspace, exists = up.Keepers.Cosmos.Params.GetSubspace(agovtypes.ModuleName)
		if !exists {
			return nil, fmt.Errorf("params subspace \"%s\" not found", agovtypes.ModuleName)
		}

		dparams := agovtypes.DepositParams{}
		sspace.Get(ctx, agovtypes.KeyDepositParams, &dparams)

		// Migrate governance min deposit parameter to builtin gov params
		gparams := up.Keepers.Cosmos.Gov.GetParams(ctx)
		gparams.MinInitialDepositRatio = dparams.MinInitialDepositRate.String()
		err = up.Keepers.Cosmos.Gov.SetParams(ctx, gparams)
		if err != nil {
			return nil, err
		}

		// // Ensure the auction module account is properly created to avoid sniping
		// err = CreateModuleAccountByName(ctx, up.Keepers.Cosmos.Acct, auctiontypes.ModuleName)
		// if err != nil {
		// 	return nil, err
		// }
		//
		// // update block-sdk params
		// if err := up.Keepers.External.Auction.SetParams(ctx, AuctionParams); err != nil {
		// 	return nil, err
		// }

		toVM, err := up.MM.RunMigrations(ctx, up.Configurator, fromVM)
		if err != nil {
			return nil, err
		}

		// patch deposit authorizations after authz store upgrade
		err = up.patchDepositAuthorizations(ctx)
		if err != nil {
			return nil, err
		}

		up.log.Info(fmt.Sprintf("migrating param astaking.MinCommissionRate to staking.MinCommissionRate"))
		sspace, exists = up.Keepers.Cosmos.Params.GetSubspace(astakingtypes.ModuleName)
		if !exists {
			return nil, fmt.Errorf("params subspace \"%s\" not found", astakingtypes.ModuleName)
		}

		sparam := sdk.Dec{}
		sspace.Get(ctx, astakingtypes.KeyMinCommissionRate, &sparam)

		sparams := up.Keepers.Cosmos.Staking.GetParams(ctx)
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
	msgUrlOld := "/akash.deployment.v1beta3.MsgDepositDeployment"

	grants := make([]grantBackup, 0, 10000)

	expiredGrants := 0

	up.Keepers.Cosmos.Authz.IterateGrants(ctx, func(granterAddr sdk.AccAddress, granteeAddr sdk.AccAddress, grant authz.Grant) bool {
		authorization, err := grant.GetAuthorization()
		if err != nil {
			up.log.Error(fmt.Sprintf("unable to get autorization. err=%s", err.Error()))
			return false
		}

		if grant.Expiration.Before(ctx.BlockHeader().Time) {
			expiredGrants++
			return false
		}

		authzOld, valid := authorization.(*dv1beta3.DepositDeploymentAuthorization)
		if !valid {
			return false
		}

		grants = append(grants, grantBackup{
			murl:       msgUrlOld,
			granter:    granterAddr,
			grantee:    granteeAddr,
			expiration: grant.Expiration,
			auth:       authzOld,
		})

		return false
	})

	err := up.Keepers.Cosmos.Authz.DequeueAndDeleteExpiredGrants(ctx, expiredGrants)
	if err != nil {
		return err
	}

	for _, grant := range grants {
		err := up.Keepers.Cosmos.Authz.DeleteGrant(ctx, grant.grantee, grant.granter, grant.murl)
		if err != nil {
			up.log.Error(fmt.Sprintf("unable to delete autorization. err=%s", err.Error()))
		}

		authzNew := dv1.NewDepositAuthorization(grant.auth.SpendLimit)
		err = up.Keepers.Cosmos.Authz.SaveGrant(ctx, grant.grantee, grant.granter, authzNew, grant.expiration)
		if err != nil {
			return err
		}
	}

	up.log.Info(fmt.Sprintf("cleaned %d expired grants", expiredGrants))

	return nil
}
