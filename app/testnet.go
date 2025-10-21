package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/codec/types"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	tmos "github.com/tendermint/tendermint/libs/os"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	utypes "github.com/akash-network/node/upgrades/types"
)

type TestnetValidator struct {
	OperatorAddress   sdk.ValAddress
	ConsensusAddress  sdk.ConsAddress
	ConsensusPubKey   *types.Any
	Status            stakingtypes.BondStatus
	Moniker           string
	Commission        stakingtypes.Commission
	MinSelfDelegation sdk.Int
	Tokens            sdk.Int
	DelegatorShares   sdk.Dec
}

type TestnetUpgrade struct {
	Name string
}

type TestnetVotingPeriod struct {
	time.Duration
}

type TestnetGovConfig struct {
	VotingParams *struct {
		VotingPeriod TestnetVotingPeriod `json:"voting_period,omitempty"`
	} `json:"voting_params,omitempty"`
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
	tcfg *TestnetConfig,
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

	ctx := app.BaseApp.NewUncachedContext(true, tmproto.Header{})

	// Remove all validators from power store
	stakingKey := app.GetKey(stakingtypes.ModuleName)
	stakingStore := ctx.KVStore(stakingKey)
	iterator := app.Keepers.Cosmos.Staking.ValidatorsPowerStoreIterator(ctx)

	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	if err := iterator.Close(); err != nil {
		panic(err)
	}

	// Remove all validators from last validators store
	iterator = app.Keepers.Cosmos.Staking.LastValidatorsIterator(ctx)

	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	if err := iterator.Close(); err != nil {
		panic(err)
	}

	// Remove all validators from validator store
	iterator = storetypes.KVStorePrefixIterator(stakingStore, stakingtypes.ValidatorsKey)
	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	if err := iterator.Close(); err != nil {
		panic(err)
	}

	// Remove all validators from unbonding queue
	iterator = storetypes.KVStorePrefixIterator(stakingStore, stakingtypes.ValidatorQueueKey)
	for ; iterator.Valid(); iterator.Next() {
		stakingStore.Delete(iterator.Key())
	}
	if err := iterator.Close(); err != nil {
		panic(err)
	}

	for _, val := range tcfg.Validators {
		// Create Validator struct for our new validator.
		newVal := stakingtypes.Validator{
			OperatorAddress: val.OperatorAddress.String(),
			ConsensusPubkey: val.ConsensusPubKey,
			Jailed:          false,
			Status:          val.Status,
			Tokens:          val.Tokens,
			DelegatorShares: val.DelegatorShares,
			Description: stakingtypes.Description{
				Moniker: val.Moniker,
			},
			Commission:        val.Commission,
			MinSelfDelegation: val.MinSelfDelegation,
		}

		// Add our validator to power and last validators store
		app.Keepers.Cosmos.Staking.SetValidator(ctx, newVal)
		err = app.Keepers.Cosmos.Staking.SetValidatorByConsAddr(ctx, newVal)
		if err != nil {
			panic(err)
		}

		app.Keepers.Cosmos.Staking.SetValidatorByPowerIndex(ctx, newVal)

		valAddr := newVal.GetOperator()
		app.Keepers.Cosmos.Staking.SetLastValidatorPower(ctx, valAddr, 0)

		app.Keepers.Cosmos.Distr.Hooks().AfterValidatorCreated(ctx, valAddr)
		app.Keepers.Cosmos.Slashing.Hooks().AfterValidatorCreated(ctx, valAddr)

		// DISTRIBUTION
		//

		// Initialize records for this validator across all distribution stores
		app.Keepers.Cosmos.Distr.SetValidatorHistoricalRewards(ctx, valAddr, 0, distrtypes.NewValidatorHistoricalRewards(sdk.DecCoins{}, 1))
		app.Keepers.Cosmos.Distr.SetValidatorCurrentRewards(ctx, valAddr, distrtypes.NewValidatorCurrentRewards(sdk.DecCoins{}, 1))
		app.Keepers.Cosmos.Distr.SetValidatorAccumulatedCommission(ctx, valAddr, distrtypes.InitialValidatorAccumulatedCommission())
		app.Keepers.Cosmos.Distr.SetValidatorOutstandingRewards(ctx, valAddr, distrtypes.ValidatorOutstandingRewards{Rewards: sdk.DecCoins{}})

		// SLASHING
		//

		newConsAddr := val.ConsensusAddress

		// Set validator signing info for our new validator.
		newValidatorSigningInfo := slashingtypes.ValidatorSigningInfo{
			Address:     newConsAddr.String(),
			StartHeight: app.LastBlockHeight() - 1,
			Tombstoned:  false,
		}

		_, err = app.Keepers.Cosmos.Staking.ApplyAndReturnValidatorSetUpdates(ctx)
		if err != nil {
			panic(err)
		}

		app.Keepers.Cosmos.Slashing.SetValidatorSigningInfo(ctx, newConsAddr, newValidatorSigningInfo)
	}

	//
	// Optional Changes:
	//

	// GOV
	//

	voteParams := app.Keepers.Cosmos.Gov.GetVotingParams(ctx)
	voteParams.VotingPeriod = tcfg.Gov.VotingParams.VotingPeriod.Duration
	app.Keepers.Cosmos.Gov.SetVotingParams(ctx, voteParams)

	// BANK
	//

	for _, account := range tcfg.Accounts {
		var coins sdk.Coins

		if len(account.Balances) > 0 {
			coins = sdk.NewCoins(account.Balances...)
		} else {
			coins = sdk.NewCoins(
				sdk.NewInt64Coin("uakt", 1000000000000),
				sdk.NewInt64Coin("ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D84", 1000000000000), // axlUSDC
			)
		}

		err := app.Keepers.Cosmos.Bank.MintCoins(ctx, minttypes.ModuleName, coins)
		if err != nil {
			panic(err)
		}
		err = app.Keepers.Cosmos.Bank.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, account.Address, coins)
		if err != nil {
			panic(err)
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

		for name, fn := range utypes.GetUpgradesList() {
			upgrade, err := fn(app.Logger(), &app.App)
			if err != nil {
				panic(err)
			}

			if tcfg.Upgrade.Name == name {
				app.Logger().Info(fmt.Sprintf("configuring upgrade `%s`", name))
				if storeUpgrades := upgrade.StoreLoader(); storeUpgrades != nil && tcfg.Upgrade.Name == name {
					app.Logger().Info(fmt.Sprintf("setting up store upgrades for `%s`", name))
					app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(app.LastBlockHeight(), storeUpgrades))
				}
			}
		}
	}

	return app
}
