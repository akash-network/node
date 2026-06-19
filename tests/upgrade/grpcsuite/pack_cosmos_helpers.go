package grpcsuite

import (
	sdkmath "cosmossdk.io/math"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
)

const wCosmosPrimaryValidator = "cosmos.primaryValidator"

func cosmosCoin(denom string, amount int64) sdk.Coin {
	return sdk.NewCoin(denom, sdkmath.NewInt(amount))
}

func cosmosCoins(denom string, amount int64) sdk.Coins {
	return sdk.NewCoins(cosmosCoin(denom, amount))
}

func (s *Suite) primaryValidator() stakingtypes.Validator {
	s.T.Helper()
	if v := s.World.Get(wCosmosPrimaryValidator); v != nil {
		return v.(stakingtypes.Validator)
	}

	q := stakingtypes.NewQueryClient(s.Conn)
	resp, err := q.Validators(s.Ctx, &stakingtypes.QueryValidatorsRequest{
		Status:     stakingtypes.BondStatusBonded,
		Pagination: &sdkquery.PageRequest{Limit: 1},
	})
	require.NoError(s.T, err, "staking Validators (bonded)")
	if len(resp.Validators) == 0 {
		resp, err = q.Validators(s.Ctx, &stakingtypes.QueryValidatorsRequest{Pagination: &sdkquery.PageRequest{Limit: 1}})
		require.NoError(s.T, err, "staking Validators")
	}
	require.NotEmpty(s.T, resp.Validators, "expected at least one staking validator")
	val := resp.Validators[0]
	s.World.Set(wCosmosPrimaryValidator, val)
	return val
}

func (s *Suite) primaryValidatorPubKey(val stakingtypes.Validator) cryptotypes.PubKey {
	s.T.Helper()
	var pk cryptotypes.PubKey
	require.NoError(s.T, s.Env.InterfaceReg.UnpackAny(val.ConsensusPubkey, &pk), "unpack validator consensus pubkey")
	return pk
}

func (s *Suite) primaryValidatorConsAddress(val stakingtypes.Validator) string {
	s.T.Helper()
	return sdk.ConsAddress(s.primaryValidatorPubKey(val).Address()).String()
}
