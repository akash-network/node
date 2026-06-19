package grpcsuite

import (
	"strings"
	"time"

	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/stretchr/testify/require"
)

type cosmosAuthVestingPack struct{}

func (cosmosAuthVestingPack) Name() string { return "cosmos-auth-vesting" }

func (cosmosAuthVestingPack) Available(d *Discovery) bool {
	return d.HasModule("cosmos.auth.v1beta1")
}

func (cosmosAuthVestingPack) Run(s *Suite) {
	q := authtypes.NewQueryClient(s.Conn)

	acctResp, err := q.Account(s.Ctx, &authtypes.QueryAccountRequest{Address: s.Env.FunderAddr.String()})
	require.NoError(s.T, err, "auth Account")
	require.NotNil(s.T, acctResp.Account, "funder account should exist")
	info, err := q.AccountInfo(s.Ctx, &authtypes.QueryAccountInfoRequest{Address: s.Env.FunderAddr.String()})
	require.NoError(s.T, err, "auth AccountInfo")
	require.Equal(s.T, s.Env.FunderAddr.String(), info.Info.Address)
	byID, err := q.AccountAddressByID(s.Ctx, &authtypes.QueryAccountAddressByIDRequest{AccountId: info.Info.AccountNumber})
	require.NoError(s.T, err, "auth AccountAddressByID")
	require.Equal(s.T, s.Env.FunderAddr.String(), byID.AccountAddress)
	accounts, err := q.Accounts(s.Ctx, &authtypes.QueryAccountsRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
	require.NoError(s.T, err, "auth Accounts")
	require.NotEmpty(s.T, accounts.Accounts, "auth Accounts should not be empty")
	_, err = q.Params(s.Ctx, &authtypes.QueryParamsRequest{})
	require.NoError(s.T, err, "auth Params")
	mods, err := q.ModuleAccounts(s.Ctx, &authtypes.QueryModuleAccountsRequest{})
	require.NoError(s.T, err, "auth ModuleAccounts")
	require.NotEmpty(s.T, mods.Accounts, "module accounts should not be empty")
	gov, err := q.ModuleAccountByName(s.Ctx, &authtypes.QueryModuleAccountByNameRequest{Name: "gov"})
	require.NoError(s.T, err, "auth ModuleAccountByName(gov)")
	require.NotNil(s.T, gov.Account, "gov module account should exist")
	prefix, err := q.Bech32Prefix(s.Ctx, &authtypes.Bech32PrefixRequest{})
	require.NoError(s.T, err, "auth Bech32Prefix")
	require.NotEmpty(s.T, prefix.Bech32Prefix)
	asBytes, err := q.AddressStringToBytes(s.Ctx, &authtypes.AddressStringToBytesRequest{
		AddressString: s.Env.FunderAddr.String(),
	})
	require.NoError(s.T, err, "auth AddressStringToBytes")
	asString, err := q.AddressBytesToString(s.Ctx, &authtypes.AddressBytesToStringRequest{AddressBytes: asBytes.AddressBytes})
	require.NoError(s.T, err, "auth AddressBytesToString")
	require.Equal(s.T, s.Env.FunderAddr.String(), asString.AddressString)
	_, err = q.AddressStringToBytes(s.Ctx, &authtypes.AddressStringToBytesRequest{AddressString: "not-an-address"})
	require.Error(s.T, err, "invalid address string should be rejected")

	now := s.LatestBlockTime()
	continuous := s.ensureKey("cosmos-vesting-continuous")
	delayed := s.ensureKey("cosmos-vesting-delayed")
	locked := s.ensureKey("cosmos-vesting-locked")
	periodic := s.ensureKey("cosmos-vesting-periodic")

	s.BroadcastOK(s.Env.Funder, vestingtypes.NewMsgCreateVestingAccount(
		s.Env.FunderAddr, continuous, cosmosCoins(s.Env.BondDenom, 1_000_000), now.Add(time.Hour).Unix(), false,
	))
	s.BroadcastOK(s.Env.Funder, vestingtypes.NewMsgCreateVestingAccount(
		s.Env.FunderAddr, delayed, cosmosCoins(s.Env.BondDenom, 1_000_000), now.Add(2*time.Hour).Unix(), true,
	))
	s.BroadcastOK(s.Env.Funder, vestingtypes.NewMsgCreatePermanentLockedAccount(
		s.Env.FunderAddr, locked, cosmosCoins(s.Env.BondDenom, 1_000_000),
	))
	s.BroadcastOK(s.Env.Funder, vestingtypes.NewMsgCreatePeriodicVestingAccount(
		s.Env.FunderAddr,
		periodic,
		now.Unix(),
		[]vestingtypes.Period{{Length: int64(time.Hour.Seconds()), Amount: cosmosCoins(s.Env.BondDenom, 1_000_000)}},
	))

	requireAccountType(s, q, continuous.String(), "ContinuousVestingAccount")
	requireAccountType(s, q, delayed.String(), "DelayedVestingAccount")
	requireAccountType(s, q, locked.String(), "PermanentLockedAccount")
	requireAccountType(s, q, periodic.String(), "PeriodicVestingAccount")

	s.BroadcastExpectErr(s.Env.Funder, vestingtypes.NewMsgCreateVestingAccount(
		s.Env.FunderAddr, continuous, cosmosCoins(s.Env.BondDenom, 1_000_000), now.Add(time.Hour).Unix(), false,
	))
	s.BroadcastExpectErr(s.Env.Funder, vestingtypes.NewMsgCreatePeriodicVestingAccount(
		s.Env.FunderAddr, s.ensureKey("cosmos-vesting-empty-periods"), now.Unix(), nil,
	))
	s.logf("cosmos auth/vesting complete (auth queries + all vesting account messages)")
}

func requireAccountType(s *Suite, q authtypes.QueryClient, addr string, want string) {
	s.T.Helper()
	resp, err := q.Account(s.Ctx, &authtypes.QueryAccountRequest{Address: addr})
	require.NoError(s.T, err, "auth Account(%s)", addr)
	require.NotNil(s.T, resp.Account, "account should exist")
	require.Truef(s.T, strings.Contains(resp.Account.TypeUrl, want), "account type %q should contain %q", resp.Account.TypeUrl, want)
}
