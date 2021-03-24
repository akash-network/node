package migrations

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingexported "github.com/cosmos/cosmos-sdk/x/auth/vesting/exported"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func MigrateAkashnet2Upgrade1(
	ctx sdk.Context,
	akeeper authkeeper.AccountKeeper,
	bkeeper bankkeeper.Keeper,
	skeeper stakingkeeper.Keeper) {

	akeeper.IterateAccounts(ctx, func(acct authtypes.AccountI) bool {
		vacct, ok := resetAccount(acct)
		if !ok {
			return false
		}

		balances := bkeeper.GetAllBalances(ctx, vacct.GetAddress())

		delegations := getDelegations(ctx, skeeper, vacct.GetAddress())

		for _, delegation := range delegations {
			balances = balances.Add(delegation)
		}

		vacct.TrackDelegation(ctx.BlockTime(), balances, delegations)

		akeeper.SetAccount(ctx, vacct)

		return false
	})
}

func getDelegations(ctx sdk.Context, skeeper stakingkeeper.Keeper, address sdk.AccAddress) sdk.Coins {
	gctx := sdk.WrapSDKContext(ctx)
	squery := stakingkeeper.Querier{skeeper}

	dresponse, err := squery.DelegatorDelegations(gctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: address.String(),
	})
	if err != nil {
		panic(fmt.Errorf("error getting delegations [%s]: %w", address, err))
	}

	delegations := sdk.NewCoins()

	for _, delegation := range dresponse.DelegationResponses {
		delegations = delegations.Add(delegation.GetBalance())
	}

	udresponse, err := squery.DelegatorUnbondingDelegations(gctx, &stakingtypes.QueryDelegatorUnbondingDelegationsRequest{
		DelegatorAddr: address.String(),
	})
	if err != nil {
		panic(fmt.Errorf("error getting delegations [%s]: %w", address, err))
	}

	denom := skeeper.BondDenom(ctx)

	for _, delegation := range udresponse.UnbondingResponses {
		for _, entry := range delegation.Entries {
			delegations = delegations.Add(sdk.NewCoin(denom, entry.Balance))
		}
	}

	return delegations
}

func resetAccount(acct authtypes.AccountI) (vestingexported.VestingAccount, bool) {
	// reset `DelegatedVesting` and `DelegatedFree` to zero
	df := sdk.NewCoins()
	dv := sdk.NewCoins()

	switch vacct := acct.(type) {
	case *vestingtypes.ContinuousVestingAccount:
		vacct.DelegatedVesting = dv
		vacct.DelegatedFree = df
		return vacct, true
	case *vestingtypes.DelayedVestingAccount:
		vacct.DelegatedVesting = dv
		vacct.DelegatedFree = df
		return vacct, true
	case *vestingtypes.PeriodicVestingAccount:
		vacct.DelegatedVesting = dv
		vacct.DelegatedFree = df
		return vacct, true
	default:
		return nil, false
	}

}
