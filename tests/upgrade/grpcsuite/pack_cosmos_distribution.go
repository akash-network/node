package grpcsuite

import (
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
)

type cosmosDistributionPack struct{}

func (cosmosDistributionPack) Name() string { return "cosmos-distribution" }

func (cosmosDistributionPack) Available(d *Discovery) bool {
	return d.HasModule("cosmos.distribution.v1beta1")
}

func (cosmosDistributionPack) Run(s *Suite) {
	q := distrtypes.NewQueryClient(s.Conn)
	val := s.primaryValidator()
	valAddr := val.OperatorAddress

	delegator := s.FundAccountDefault("cosmos-distr-delegator")
	withdrawAddr := s.ensureKey("cosmos-distr-withdraw")
	s.BroadcastOK("cosmos-distr-delegator", stakingtypes.NewMsgDelegate(
		delegator.String(), valAddr, cosmosCoin(s.Env.BondDenom, 5_000_000),
	))
	s.WaitBlocks(1)

	s.BroadcastOK("cosmos-distr-delegator", distrtypes.NewMsgSetWithdrawAddress(delegator, withdrawAddr))
	waddr, err := q.DelegatorWithdrawAddress(s.Ctx, &distrtypes.QueryDelegatorWithdrawAddressRequest{
		DelegatorAddress: delegator.String(),
	})
	require.NoError(s.T, err, "distribution DelegatorWithdrawAddress")
	require.Equal(s.T, withdrawAddr.String(), waddr.WithdrawAddress)

	s.BroadcastOK("cosmos-distr-delegator", distrtypes.NewMsgFundCommunityPool(
		cosmosCoins(s.Env.BondDenom, 2_000_000), delegator.String(),
	))
	s.BroadcastOK("cosmos-distr-delegator", distrtypes.NewMsgDepositValidatorRewardsPool(
		delegator.String(), valAddr, cosmosCoins(s.Env.BondDenom, 1_000_000),
	))
	s.BroadcastOK("cosmos-distr-delegator", distrtypes.NewMsgWithdrawDelegatorReward(delegator.String(), valAddr))
	s.BroadcastTolerant(s.Env.Funder, []string{"no validator commission to withdraw"},
		distrtypes.NewMsgWithdrawValidatorCommission(valAddr))

	recipient := s.ensureKey("cosmos-distr-community-spend")
	s.PassGovProposal("grpcsuite: distribution community pool spend",
		&distrtypes.MsgCommunityPoolSpend{
			Authority: s.GovAuthority(),
			Recipient: recipient.String(),
			Amount:    cosmosCoins(s.Env.BondDenom, 1_000_000),
		},
	)

	_, err = q.Params(s.Ctx, &distrtypes.QueryParamsRequest{})
	require.NoError(s.T, err, "distribution Params")
	_, err = q.ValidatorDistributionInfo(s.Ctx, &distrtypes.QueryValidatorDistributionInfoRequest{ValidatorAddress: valAddr})
	require.NoError(s.T, err, "distribution ValidatorDistributionInfo")
	_, err = q.ValidatorOutstandingRewards(s.Ctx, &distrtypes.QueryValidatorOutstandingRewardsRequest{ValidatorAddress: valAddr})
	require.NoError(s.T, err, "distribution ValidatorOutstandingRewards")
	_, err = q.ValidatorCommission(s.Ctx, &distrtypes.QueryValidatorCommissionRequest{ValidatorAddress: valAddr})
	require.NoError(s.T, err, "distribution ValidatorCommission")
	_, err = q.ValidatorSlashes(s.Ctx, &distrtypes.QueryValidatorSlashesRequest{
		ValidatorAddress: valAddr,
		StartingHeight:   1,
		EndingHeight:     uint64(s.LatestHeight()),
		Pagination:       &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "distribution ValidatorSlashes")
	_, err = q.DelegationRewards(s.Ctx, &distrtypes.QueryDelegationRewardsRequest{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: valAddr,
	})
	require.NoError(s.T, err, "distribution DelegationRewards")
	total, err := q.DelegationTotalRewards(s.Ctx, &distrtypes.QueryDelegationTotalRewardsRequest{
		DelegatorAddress: delegator.String(),
	})
	require.NoError(s.T, err, "distribution DelegationTotalRewards")
	require.NotNil(s.T, total, "total rewards response")
	dvals, err := q.DelegatorValidators(s.Ctx, &distrtypes.QueryDelegatorValidatorsRequest{
		DelegatorAddress: delegator.String(),
	})
	require.NoError(s.T, err, "distribution DelegatorValidators")
	require.NotEmpty(s.T, dvals.Validators, "delegator should have validator rewards list")
	pool, err := q.CommunityPool(s.Ctx, &distrtypes.QueryCommunityPoolRequest{})
	require.NoError(s.T, err, "distribution CommunityPool")
	require.NotNil(s.T, pool, "community pool response")

	s.BroadcastExpectErr(s.Env.Funder, distrtypes.NewMsgSetWithdrawAddress(delegator, s.Env.FunderAddr))
	s.BroadcastExpectErr("cosmos-distr-delegator", distrtypes.NewMsgWithdrawDelegatorReward(delegator.String(), "akashvaloper1deadbeef"))
	s.logf("cosmos distribution complete (withdraw address, rewards, community pool)")
}
