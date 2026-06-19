package grpcsuite

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
)

type cosmosStakingPack struct{}

func (cosmosStakingPack) Name() string { return "cosmos-staking" }

func (cosmosStakingPack) Available(d *Discovery) bool { return d.HasModule("cosmos.staking.v1beta1") }

func (cosmosStakingPack) Run(s *Suite) {
	q := stakingtypes.NewQueryClient(s.Conn)
	val := s.primaryValidator()
	valAddr := val.OperatorAddress

	validators, err := q.Validators(s.Ctx, &stakingtypes.QueryValidatorsRequest{
		Status:     stakingtypes.BondStatusBonded,
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "staking Validators")
	require.NotEmpty(s.T, validators.Validators, "expected bonded validators")
	one, err := q.Validator(s.Ctx, &stakingtypes.QueryValidatorRequest{ValidatorAddr: valAddr})
	require.NoError(s.T, err, "staking Validator")
	require.Equal(s.T, valAddr, one.Validator.OperatorAddress)
	_, err = q.Pool(s.Ctx, &stakingtypes.QueryPoolRequest{})
	require.NoError(s.T, err, "staking Pool")
	_, err = q.Params(s.Ctx, &stakingtypes.QueryParamsRequest{})
	require.NoError(s.T, err, "staking Params")

	dup, err := stakingtypes.NewMsgCreateValidator(
		valAddr,
		s.primaryValidatorPubKey(val),
		cosmosCoin(s.Env.BondDenom, 1),
		stakingtypes.NewDescription("duplicate", "", "", "", ""),
		val.Commission.CommissionRates,
		sdkmath.NewInt(1),
	)
	require.NoError(s.T, err, "build duplicate MsgCreateValidator")
	s.BroadcastExpectErr(s.Env.Funder, dup)

	edit := stakingtypes.NewMsgEditValidator(
		valAddr,
		stakingtypes.NewDescription(fmt.Sprintf("grpcsuite-%d", s.LatestHeight()), "", "", "", ""),
		nil,
		nil,
	)
	s.BroadcastOK(s.Env.Funder, edit)

	delegator := s.FundAccountDefault("cosmos-staking-delegator")
	delegationAmt := cosmosCoin(s.Env.BondDenom, 10_000_000)
	s.BroadcastOK("cosmos-staking-delegator", stakingtypes.NewMsgDelegate(delegator.String(), valAddr, delegationAmt))

	del, err := q.Delegation(s.Ctx, &stakingtypes.QueryDelegationRequest{
		DelegatorAddr: delegator.String(),
		ValidatorAddr: valAddr,
	})
	require.NoError(s.T, err, "staking Delegation")
	require.Equal(s.T, delegator.String(), del.DelegationResponse.Delegation.DelegatorAddress)
	valDels, err := q.ValidatorDelegations(s.Ctx, &stakingtypes.QueryValidatorDelegationsRequest{
		ValidatorAddr: valAddr,
		Pagination:    &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "staking ValidatorDelegations")
	require.NotEmpty(s.T, valDels.DelegationResponses, "validator should have delegations")
	delDels, err := q.DelegatorDelegations(s.Ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delegator.String(),
		Pagination:    &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "staking DelegatorDelegations")
	require.NotEmpty(s.T, delDels.DelegationResponses, "delegator should have delegations")
	delVals, err := q.DelegatorValidators(s.Ctx, &stakingtypes.QueryDelegatorValidatorsRequest{
		DelegatorAddr: delegator.String(),
		Pagination:    &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "staking DelegatorValidators")
	require.NotEmpty(s.T, delVals.Validators, "delegator should have validator list")
	delVal, err := q.DelegatorValidator(s.Ctx, &stakingtypes.QueryDelegatorValidatorRequest{
		DelegatorAddr: delegator.String(),
		ValidatorAddr: valAddr,
	})
	require.NoError(s.T, err, "staking DelegatorValidator")
	require.Equal(s.T, valAddr, delVal.Validator.OperatorAddress)

	s.BroadcastExpectErr("cosmos-staking-delegator", stakingtypes.NewMsgBeginRedelegate(
		delegator.String(), valAddr, valAddr, cosmosCoin(s.Env.BondDenom, 1_000_000),
	))
	_, err = q.Redelegations(s.Ctx, &stakingtypes.QueryRedelegationsRequest{
		DelegatorAddr:    delegator.String(),
		SrcValidatorAddr: valAddr,
		DstValidatorAddr: valAddr,
		Pagination:       &sdkquery.PageRequest{Limit: 10},
	})
	if err != nil {
		require.Contains(s.T, err.Error(), "redelegation not found", "staking Redelegations")
	}

	unbondAmt := cosmosCoin(s.Env.BondDenom, 3_000_000)
	s.BroadcastOK("cosmos-staking-delegator", stakingtypes.NewMsgUndelegate(delegator.String(), valAddr, unbondAmt))
	unbond, err := q.UnbondingDelegation(s.Ctx, &stakingtypes.QueryUnbondingDelegationRequest{
		DelegatorAddr: delegator.String(),
		ValidatorAddr: valAddr,
	})
	require.NoError(s.T, err, "staking UnbondingDelegation")
	require.NotEmpty(s.T, unbond.Unbond.Entries, "expected an unbonding entry")
	creationHeight := unbond.Unbond.Entries[0].CreationHeight
	_, err = q.ValidatorUnbondingDelegations(s.Ctx, &stakingtypes.QueryValidatorUnbondingDelegationsRequest{
		ValidatorAddr: valAddr,
		Pagination:    &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "staking ValidatorUnbondingDelegations")
	_, err = q.DelegatorUnbondingDelegations(s.Ctx, &stakingtypes.QueryDelegatorUnbondingDelegationsRequest{
		DelegatorAddr: delegator.String(),
		Pagination:    &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "staking DelegatorUnbondingDelegations")
	s.BroadcastOK("cosmos-staking-delegator", stakingtypes.NewMsgCancelUnbondingDelegation(
		delegator.String(), valAddr, creationHeight, unbondAmt,
	))
	_, _ = q.HistoricalInfo(s.Ctx, &stakingtypes.QueryHistoricalInfoRequest{Height: s.LatestHeight() - 1})
	s.logf("cosmos staking complete (edit, delegate, undelegate, cancel-unbonding, duplicate/redelegate edges)")
}
