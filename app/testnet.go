package app

import (
	"fmt"
	"time"

	tmos "github.com/cometbft/cometbft/libs/os"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	appparams "pkg.akt.dev/node/app/params"
	utypes "pkg.akt.dev/node/upgrades/types"
)

type TestnetValidator struct {
	OperatorAddress   sdk.Address
	ConsensusAddress  sdk.ConsAddress
	ConsensusPubKey   *types.Any
	Moniker           string
	Commission        stakingtypes.Commission
	MinSelfDelegation sdkmath.Int
}

type TestnetGov struct {
	VotePeriod          time.Duration `json:"vote_period"`
	ExpeditedVotePeriod time.Duration `json:"expedited_vote_period"`
}

type TestnetUpgrade struct {
	Name string
}

type TestnetConfig struct {
	Accounts   []sdk.AccAddress
	Validators []TestnetValidator
	Gov        TestnetGov
	Upgrade    TestnetUpgrade
}

// InitAkashAppForTestnet is broken down into two sections:
// Required Changes: Changes that, if not made, will cause the testnet to halt or panic
// Optional Changes: Changes to customize the testnet to one's liking (lower vote times, fund accounts, etc)
func InitAkashAppForTestnet(
	app *AkashApp,
	db dbm.DB,
	tcfg TestnetConfig,
) *AkashApp {
	//
	// Required Changes:
	//

	var err error

	defer func() {
		if err != nil {
			tmos.Exit(err.Error())
		}
	}()

	ctx := app.NewUncachedContext(true, tmproto.Header{})

	// STAKING
	//

	// Remove all validators from power store
	stakingKey := app.GetKey(stakingtypes.ModuleName)
	stakingStore := ctx.KVStore(stakingKey)
	iterator, err := app.Keepers.Cosmos.Staking.ValidatorsPowerStoreIterator(ctx)
	if err != nil {
		return nil
	}
	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	_ = iterator.Close()

	// Remove all validators from last validators store
	iterator, err = app.Keepers.Cosmos.Staking.LastValidatorsIterator(ctx)
	if err != nil {
		return nil
	}
	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	_ = iterator.Close()

	// Remove all validators from validator store
	iterator = storetypes.KVStorePrefixIterator(stakingStore, stakingtypes.ValidatorsKey)
	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	_ = iterator.Close()

	// Remove all validators from unbonding queue
	iterator = storetypes.KVStorePrefixIterator(stakingStore, stakingtypes.ValidatorQueueKey)
	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	_ = iterator.Close()

	for _, val := range tcfg.Validators {
		_, bz, err := bech32.DecodeAndConvert(val.OperatorAddress.String())
		if err != nil {
			return nil
		}
		bech32Addr, err := bech32.ConvertAndEncode("akashvaloper", bz)
		if err != nil {
			return nil
		}

		// Create Validator struct for our new validator.
		newVal := stakingtypes.Validator{
			OperatorAddress: bech32Addr,
			ConsensusPubkey: val.ConsensusPubKey,
			Jailed:          false,
			Status:          stakingtypes.Bonded,
			Tokens:          sdkmath.NewInt(900000000000000),
			DelegatorShares: sdkmath.LegacyMustNewDecFromStr("10000000"),
			Description: stakingtypes.Description{
				Moniker: "Testnet Validator",
			},
			Commission: stakingtypes.Commission{
				CommissionRates: stakingtypes.CommissionRates{
					Rate:          sdkmath.LegacyMustNewDecFromStr("0.05"),
					MaxRate:       sdkmath.LegacyMustNewDecFromStr("0.1"),
					MaxChangeRate: sdkmath.LegacyMustNewDecFromStr("0.05"),
				},
			},
			MinSelfDelegation: sdkmath.OneInt(),
		}

		// Add our validator to power and last validators store
		err = app.Keepers.Cosmos.Staking.SetValidator(ctx, newVal)
		if err != nil {
			return nil
		}
		err = app.Keepers.Cosmos.Staking.SetValidatorByConsAddr(ctx, newVal)
		if err != nil {
			return nil
		}
		err = app.Keepers.Cosmos.Staking.SetValidatorByPowerIndex(ctx, newVal)
		if err != nil {
			return nil
		}
		valAddr, err := sdk.ValAddressFromBech32(newVal.GetOperator())
		if err != nil {
			return nil
		}
		err = app.Keepers.Cosmos.Staking.SetLastValidatorPower(ctx, valAddr, 0)
		if err != nil {
			return nil
		}
		if err := app.Keepers.Cosmos.Staking.Hooks().AfterValidatorCreated(ctx, valAddr); err != nil {
			panic(err)
		}

		// DISTRIBUTION
		//

		// Initialize records for this validator across all distribution stores
		valAddr, err = sdk.ValAddressFromBech32(newVal.GetOperator())
		if err != nil {
			return nil
		}
		err = app.Keepers.Cosmos.Distr.SetValidatorHistoricalRewards(ctx, valAddr, 0, distrtypes.NewValidatorHistoricalRewards(sdk.DecCoins{}, 1))
		if err != nil {
			return nil
		}
		err = app.Keepers.Cosmos.Distr.SetValidatorCurrentRewards(ctx, valAddr, distrtypes.NewValidatorCurrentRewards(sdk.DecCoins{}, 1))
		if err != nil {
			return nil
		}
		err = app.Keepers.Cosmos.Distr.SetValidatorAccumulatedCommission(ctx, valAddr, distrtypes.InitialValidatorAccumulatedCommission())
		if err != nil {
			return nil
		}
		err = app.Keepers.Cosmos.Distr.SetValidatorOutstandingRewards(ctx, valAddr, distrtypes.ValidatorOutstandingRewards{Rewards: sdk.DecCoins{}})
		if err != nil {
			return nil
		}

		// SLASHING
		//

		newConsAddr := val.ConsensusAddress
		// Set validator signing info for our new validator.
		newValidatorSigningInfo := slashingtypes.ValidatorSigningInfo{
			Address:     newConsAddr.String(),
			StartHeight: app.LastBlockHeight() - 1,
			Tombstoned:  false,
		}
		err = app.Keepers.Cosmos.Slashing.SetValidatorSigningInfo(ctx, newConsAddr, newValidatorSigningInfo)
		if err != nil {
			return nil
		}
	}
	//
	// Optional Changes:
	//

	// GOV
	//

	govParams, err := app.Keepers.Cosmos.Gov.Params.Get(ctx)
	if err != nil {
		return nil
	}
	govParams.ExpeditedVotingPeriod = &tcfg.Gov.ExpeditedVotePeriod
	govParams.VotingPeriod = &tcfg.Gov.VotePeriod
	govParams.MinDeposit = sdk.NewCoins(sdk.NewInt64Coin(appparams.BaseCoinUnit, 100000000))
	govParams.ExpeditedMinDeposit = sdk.NewCoins(sdk.NewInt64Coin(appparams.BaseCoinUnit, 150000000))

	err = app.Keepers.Cosmos.Gov.Params.Set(ctx, govParams)
	if err != nil {
		return nil
	}

	// BANK
	//

	defaultCoins := sdk.NewCoins(
		sdk.NewInt64Coin("uakt", 1000000000000),
		sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D84", 1000000000000), // axlUSDC
	)

	// Fund localakash accounts
	for _, account := range tcfg.Accounts {
		err := app.Keepers.Cosmos.Bank.MintCoins(ctx, minttypes.ModuleName, defaultCoins)
		if err != nil {
			return nil
		}
		err = app.Keepers.Cosmos.Bank.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, account, defaultCoins)
		if err != nil {
			return nil
		}
	}

	// UPGRADE
	//
	if tcfg.Upgrade.Name != "" {
		upgradePlan := upgradetypes.Plan{
			Name:   tcfg.Upgrade.Name,
			Height: app.LastBlockHeight() + 10,
		}

		err = app.Keepers.Cosmos.Upgrade.ScheduleUpgrade(ctx, upgradePlan)
		if err != nil {
			panic(err)
		}

		version := store.NewCommitMultiStore(db, log.NewNopLogger(), nil).LatestVersion() + 1

		for name, fn := range utypes.GetUpgradesList() {
			upgrade, err := fn(app.Log, app.App)
			if err != nil {
				panic(err)
			}

			if tcfg.Upgrade.Name == name {
				app.Log.Info(fmt.Sprintf("configuring upgrade `%s`", name))
				if storeUpgrades := upgrade.StoreLoader(); storeUpgrades != nil && tcfg.Upgrade.Name == name {
					app.Log.Info(fmt.Sprintf("setting up store upgrades for `%s`", name))
					app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(version, storeUpgrades))
				}
			}
		}
	}

	return app
}
