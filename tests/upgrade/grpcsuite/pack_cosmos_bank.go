package grpcsuite

import (
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type cosmosBankPack struct{}

func (cosmosBankPack) Name() string { return "cosmos-bank" }

func (cosmosBankPack) Available(d *Discovery) bool { return d.HasModule("cosmos.bank.v1beta1") }

func (cosmosBankPack) Run(s *Suite) {
	q := banktypes.NewQueryClient(s.Conn)

	sender := s.FundAccountDefault("cosmos-bank-sender")
	recvA := s.ensureKey("cosmos-bank-recv-a")
	recvB := s.ensureKey("cosmos-bank-recv-b")

	sendAmt := cosmosCoins(s.Env.BondDenom, 1_000_000)
	before := s.balanceOf(recvA, s.Env.BondDenom)
	s.BroadcastOK("cosmos-bank-sender", banktypes.NewMsgSend(sender, recvA, sendAmt))
	after := s.balanceOf(recvA, s.Env.BondDenom)
	require.Equal(s.T, sendAmt[0].Amount.String(), after.Amount.Sub(before.Amount).String(), "MsgSend receiver delta")

	s.BroadcastOK("cosmos-bank-sender", banktypes.NewMsgMultiSend(
		banktypes.Input{Address: sender.String(), Coins: cosmosCoins(s.Env.BondDenom, 2_000_000)},
		[]banktypes.Output{
			{Address: recvA.String(), Coins: cosmosCoins(s.Env.BondDenom, 1_000_000)},
			{Address: recvB.String(), Coins: cosmosCoins(s.Env.BondDenom, 1_000_000)},
		},
	))

	bal, err := q.Balance(s.Ctx, &banktypes.QueryBalanceRequest{Address: recvA.String(), Denom: s.Env.BondDenom})
	require.NoError(s.T, err, "bank Balance")
	require.True(s.T, bal.Balance.Amount.IsPositive(), "receiver balance should be positive")
	all, err := q.AllBalances(s.Ctx, &banktypes.QueryAllBalancesRequest{
		Address:    recvA.String(),
		Pagination: &sdkquery.PageRequest{Limit: 1},
	})
	require.NoError(s.T, err, "bank AllBalances")
	require.LessOrEqual(s.T, len(all.Balances), 1, "AllBalances pagination Limit=1 must cap results")
	spendable, err := q.SpendableBalances(s.Ctx, &banktypes.QuerySpendableBalancesRequest{Address: recvA.String()})
	require.NoError(s.T, err, "bank SpendableBalances")
	require.True(s.T, spendable.Balances.AmountOf(s.Env.BondDenom).IsPositive(), "receiver should have spendable uakt")
	spendableDenom, err := q.SpendableBalanceByDenom(s.Ctx, &banktypes.QuerySpendableBalanceByDenomRequest{
		Address: recvA.String(),
		Denom:   s.Env.BondDenom,
	})
	require.NoError(s.T, err, "bank SpendableBalanceByDenom")
	require.True(s.T, spendableDenom.Balance.Amount.IsPositive(), "receiver should have spendable denom balance")
	supply, err := q.TotalSupply(s.Ctx, &banktypes.QueryTotalSupplyRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
	require.NoError(s.T, err, "bank TotalSupply")
	require.NotEmpty(s.T, supply.Supply, "total supply should not be empty")
	supplyOf, err := q.SupplyOf(s.Ctx, &banktypes.QuerySupplyOfRequest{Denom: s.Env.BondDenom})
	require.NoError(s.T, err, "bank SupplyOf")
	require.Equal(s.T, s.Env.BondDenom, supplyOf.Amount.Denom)
	_, err = q.Params(s.Ctx, &banktypes.QueryParamsRequest{})
	require.NoError(s.T, err, "bank Params")
	_, err = q.DenomsMetadata(s.Ctx, &banktypes.QueryDenomsMetadataRequest{Pagination: &sdkquery.PageRequest{Limit: 10}})
	require.NoError(s.T, err, "bank DenomsMetadata")
	metadata, err := q.DenomMetadata(s.Ctx, &banktypes.QueryDenomMetadataRequest{Denom: s.Env.BondDenom})
	requireBankMetadataResult(s, "bank DenomMetadata", err)
	if err == nil {
		require.Equal(s.T, s.Env.BondDenom, metadata.Metadata.Base, "metadata base denom should match")
	}
	metadataByQuery, err := q.DenomMetadataByQueryString(s.Ctx, &banktypes.QueryDenomMetadataByQueryStringRequest{Denom: s.Env.BondDenom})
	requireBankMetadataResult(s, "bank DenomMetadataByQueryString", err)
	if err == nil {
		require.Equal(s.T, s.Env.BondDenom, metadataByQuery.Metadata.Base, "metadata query base denom should match")
	}
	owners, err := q.DenomOwners(s.Ctx, &banktypes.QueryDenomOwnersRequest{
		Denom:      s.Env.BondDenom,
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "bank DenomOwners")
	require.NotEmpty(s.T, owners.DenomOwners, "bond denom should have owners")
	_, err = q.DenomOwnersByQuery(s.Ctx, &banktypes.QueryDenomOwnersByQueryRequest{
		Denom:      s.Env.BondDenom,
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "bank DenomOwnersByQuery")
	_, err = q.SendEnabled(s.Ctx, &banktypes.QuerySendEnabledRequest{Denoms: []string{s.Env.BondDenom}})
	require.NoError(s.T, err, "bank SendEnabled")

	poor := s.FundAccount("cosmos-bank-poor", cosmosCoins(s.Env.BondDenom, 2_000_000))
	s.BroadcastExpectErr("cosmos-bank-poor", banktypes.NewMsgSend(poor, recvB, cosmosCoins(s.Env.BondDenom, 1_000_000_000_000)))
	s.BroadcastExpectErr("cosmos-bank-sender", banktypes.NewMsgSend(sender, recvA, cosmosCoins(s.Env.BondDenom, 0)))
	s.BroadcastExpectErr("cosmos-bank-sender", banktypes.NewMsgMultiSend(
		banktypes.Input{Address: sender.String(), Coins: cosmosCoins(s.Env.BondDenom, 1_000_000)},
		[]banktypes.Output{{Address: recvB.String(), Coins: cosmosCoins(s.Env.BondDenom, 2_000_001)}},
	))

	s.logf("cosmos bank complete (send, multisend, typed queries, negative sends)")
}

func requireBankMetadataResult(s *Suite, query string, err error) {
	s.T.Helper()
	if err == nil {
		return
	}
	require.Equal(s.T, codes.NotFound, status.Code(err), "%s should only fail when metadata is absent", query)
}
