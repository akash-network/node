package grpcsuite

import (
	"time"

	feegrant "cosmossdk.io/x/feegrant"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
)

type cosmosFeegrantPack struct{}

func (cosmosFeegrantPack) Name() string { return "cosmos-feegrant" }

func (cosmosFeegrantPack) Available(d *Discovery) bool { return d.HasModule("cosmos.feegrant.v1beta1") }

func (cosmosFeegrantPack) Run(s *Suite) {
	q := feegrant.NewQueryClient(s.Conn)

	granter := s.FundAccountDefault("cosmos-feegrant-granter")
	grantee := s.FundAccountDefault("cosmos-feegrant-grantee")
	exp := s.LatestBlockTime().Add(time.Hour)
	msg, err := feegrant.NewMsgGrantAllowance(&feegrant.BasicAllowance{
		SpendLimit: cosmosCoins(s.Env.BondDenom, 10_000_000),
		Expiration: &exp,
	}, granter, grantee)
	require.NoError(s.T, err, "build feegrant MsgGrantAllowance")
	s.BroadcastOK("cosmos-feegrant-granter", msg)

	allowance, err := q.Allowance(s.Ctx, &feegrant.QueryAllowanceRequest{Granter: granter.String(), Grantee: grantee.String()})
	require.NoError(s.T, err, "feegrant Allowance")
	require.NotNil(s.T, allowance.Allowance, "expected fee allowance")
	byGrantee, err := q.Allowances(s.Ctx, &feegrant.QueryAllowancesRequest{
		Grantee:    grantee.String(),
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "feegrant Allowances")
	require.NotEmpty(s.T, byGrantee.Allowances, "grantee should have fee allowances")
	byGranter, err := q.AllowancesByGranter(s.Ctx, &feegrant.QueryAllowancesByGranterRequest{
		Granter:    granter.String(),
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "feegrant AllowancesByGranter")
	require.NotEmpty(s.T, byGranter.Allowances, "granter should have fee allowances")

	dup, err := feegrant.NewMsgGrantAllowance(&feegrant.BasicAllowance{
		SpendLimit: cosmosCoins(s.Env.BondDenom, 1_000_000),
		Expiration: &exp,
	}, granter, grantee)
	require.NoError(s.T, err, "build duplicate feegrant allowance")
	s.BroadcastExpectErr("cosmos-feegrant-granter", dup)

	revoke := feegrant.NewMsgRevokeAllowance(granter, grantee)
	s.BroadcastOK("cosmos-feegrant-granter", &revoke)
	_, err = q.Allowance(s.Ctx, &feegrant.QueryAllowanceRequest{Granter: granter.String(), Grantee: grantee.String()})
	require.Error(s.T, err, "feegrant Allowance should fail after revoke")
	missing := feegrant.NewMsgRevokeAllowance(granter, grantee)
	s.BroadcastExpectErr("cosmos-feegrant-granter", &missing)

	pruner := s.FundAccountDefault("cosmos-feegrant-pruner")
	s.BroadcastOK("cosmos-feegrant-pruner", &feegrant.MsgPruneAllowances{Pruner: pruner.String()})
	s.logf("cosmos feegrant complete (grant, revoke, prune, duplicate/missing edges)")
}
