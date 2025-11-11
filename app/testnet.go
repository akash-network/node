package app

import (
	"fmt"
	"strings"
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

	"pkg.akt.dev/go/sdkutil"

	utypes "pkg.akt.dev/node/upgrades/types"
)

type TestnetDelegation struct {
	Address sdk.AccAddress `json:"address"`
	Amount  sdk.Coin       `json:"amount"`
}

type TestnetValidator struct {
	OperatorAddress   sdk.Address
	ConsensusAddress  sdk.ConsAddress
	ConsensusPubKey   *types.Any
	Status            stakingtypes.BondStatus
	Moniker           string
	Commission        stakingtypes.Commission
	MinSelfDelegation sdkmath.Int
	Delegations       []TestnetDelegation
}

type TestnetVotingPeriod struct {
	time.Duration
}

type TestnetGovConfig struct {
	VotingParams *struct {
		VotingPeriod        TestnetVotingPeriod `json:"voting_period,omitempty"`
		ExpeditedVotePeriod TestnetVotingPeriod `json:"expedited_vote_period"`
	} `json:"voting_params,omitempty"`
}

type TestnetUpgrade struct {
	Name string
}

type TestnetAccount struct {
	Address  sdk.AccAddress `json:"address"`
	Balances []sdk.Coin     `json:"balances"`
}

type TestnetConfig struct {
	Accounts   []TestnetAccount
	Validators []TestnetValidator
	Gov        TestnetGovConfig
	Upgrade    TestnetUpgrade
}

func TrimQuotes(data string) string {
	data = strings.TrimPrefix(data, "\"")
	return strings.TrimSuffix(data, "\"")
}

func (t *TestnetVotingPeriod) UnmarshalJSON(data []byte) error {
	val := TrimQuotes(string(data))

	if !strings.HasSuffix(val, "s") {
		return fmt.Errorf("invalid format of voting period. must contain time unit. Valid time units are ns|us(µs)|ms|s|m|h") // nolint: goerr113
	}

	var err error
	t.Duration, err = time.ParseDuration(val)
	if err != nil {
		return err
	}

	return nil
}

// InitAkashAppForTestnet is broken down into two sections:
// Required Changes: Changes that, if not made, will cause the testnet to halt or panic
// Optional Changes: Changes to customize the testnet to one's liking (lower vote times, fund accounts, etc)
func InitAkashAppForTestnet(
	app *AkashApp,
	db dbm.DB,
	tcfg *TestnetConfig,
) *AkashApp {
	//
	// Required Changes:
	//

	if tcfg == nil {
		tmos.Exit("TestnetConfig cannot be nil")
	}

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
		panic(err.Error())
	}
	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	_ = iterator.Close()

	// Remove all validators from the last validators store
	iterator, err = app.Keepers.Cosmos.Staking.LastValidatorsIterator(ctx)
	if err != nil {
		return nil
	}
	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	_ = iterator.Close()

	// Remove all validators from the validator store
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

	// BANK
	//

	// Fund localakash accounts
	for _, account := range tcfg.Accounts {
		err := app.Keepers.Cosmos.Bank.MintCoins(ctx, minttypes.ModuleName, account.Balances)
		if err != nil {
			panic(err.Error())
		}
		err = app.Keepers.Cosmos.Bank.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, account.Address, account.Balances)
		if err != nil {
			panic(err.Error())
		}
	}

	for _, val := range tcfg.Validators {
		_, bz, err := bech32.DecodeAndConvert(val.OperatorAddress.String())
		if err != nil {
			panic(err.Error())
		}
		bech32Addr, err := bech32.ConvertAndEncode("akashvaloper", bz)
		if err != nil {
			panic(err.Error())
		}

		// Create Validator struct for our new validator.
		newVal := stakingtypes.Validator{
			OperatorAddress: bech32Addr,
			ConsensusPubkey: val.ConsensusPubKey,
			Jailed:          false,
			Status:          stakingtypes.Bonded,
			Tokens:          sdkmath.NewInt(0),
			DelegatorShares: sdkmath.LegacyMustNewDecFromStr("0"),
			Description: stakingtypes.Description{
				Moniker: val.Moniker,
			},
			Commission:        val.Commission,
			MinSelfDelegation: val.MinSelfDelegation,
		}

		// Add our validator to power and last validators store
		err = app.Keepers.Cosmos.Staking.SetValidator(ctx, newVal)
		if err != nil {
			panic(err.Error())
		}
		err = app.Keepers.Cosmos.Staking.SetValidatorByConsAddr(ctx, newVal)
		if err != nil {
			panic(err.Error())
		}
		err = app.Keepers.Cosmos.Staking.SetValidatorByPowerIndex(ctx, newVal)
		if err != nil {
			panic(err.Error())
		}
		valAddr, err := sdk.ValAddressFromBech32(newVal.GetOperator())
		if err != nil {
			panic(err.Error())
		}
		err = app.Keepers.Cosmos.Staking.SetLastValidatorPower(ctx, valAddr, 0)
		if err != nil {
			panic(err.Error())
		}
		if err := app.Keepers.Cosmos.Staking.Hooks().AfterValidatorCreated(ctx, valAddr); err != nil {
			panic(err)
		}

		// DISTRIBUTION
		//

		// Initialize records for this validator across all distribution stores
		valAddr, err = sdk.ValAddressFromBech32(newVal.GetOperator())
		if err != nil {
			panic(err.Error())
		}
		err = app.Keepers.Cosmos.Distr.SetValidatorHistoricalRewards(ctx, valAddr, 0, distrtypes.NewValidatorHistoricalRewards(sdk.DecCoins{}, 1))
		if err != nil {
			panic(err.Error())
		}
		err = app.Keepers.Cosmos.Distr.SetValidatorCurrentRewards(ctx, valAddr, distrtypes.NewValidatorCurrentRewards(sdk.DecCoins{}, 1))
		if err != nil {
			panic(err.Error())
		}
		err = app.Keepers.Cosmos.Distr.SetValidatorAccumulatedCommission(ctx, valAddr, distrtypes.InitialValidatorAccumulatedCommission())
		if err != nil {
			panic(err.Error())
		}
		err = app.Keepers.Cosmos.Distr.SetValidatorOutstandingRewards(ctx, valAddr, distrtypes.ValidatorOutstandingRewards{Rewards: sdk.DecCoins{}})
		if err != nil {
			panic(err.Error())
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
			panic(err.Error())
		}

		for _, del := range val.Delegations {
			vl, err := app.Keepers.Cosmos.Staking.GetValidator(ctx, valAddr)
			if err != nil {
				panic(err.Error())
			}

			_, err = app.Keepers.Cosmos.Staking.Delegate(ctx, del.Address, del.Amount.Amount, stakingtypes.Unbonded, vl, true)
			if err != nil {
				panic(err)
			}
		}
	}
	//
	// Optional Changes:
	//

	// GOV
	//

	govParams, err := app.Keepers.Cosmos.Gov.Params.Get(ctx)
	if err != nil {
		panic(err.Error())
	}
	govParams.ExpeditedVotingPeriod = &tcfg.Gov.VotingParams.ExpeditedVotePeriod.Duration
	govParams.VotingPeriod = &tcfg.Gov.VotingParams.VotingPeriod.Duration
	govParams.MinDeposit = sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 100000000))
	govParams.ExpeditedMinDeposit = sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 150000000))

	err = app.Keepers.Cosmos.Gov.Params.Set(ctx, govParams)
	if err != nil {
		panic(err.Error())
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
