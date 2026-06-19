package grpcsuite

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
)

type cosmosAuthzPack struct{}

func (cosmosAuthzPack) Name() string { return "cosmos-authz" }

func (cosmosAuthzPack) Available(d *Discovery) bool { return d.HasModule("cosmos.authz.v1beta1") }

func (cosmosAuthzPack) Run(s *Suite) {
	q := authz.NewQueryClient(s.Conn)

	granter := s.FundAccountDefault("cosmos-authz-granter")
	grantee := s.FundAccountDefault("cosmos-authz-grantee")
	receiver := s.ensureKey("cosmos-authz-receiver")
	msgType := "/cosmos.bank.v1beta1.MsgSend"
	exp := s.LatestBlockTime().Add(time.Hour)

	grant, err := authz.NewMsgGrant(
		granter,
		grantee,
		banktypes.NewSendAuthorization(cosmosCoins(s.Env.BondDenom, 3_000_000), []sdk.AccAddress{receiver}),
		&exp,
	)
	require.NoError(s.T, err, "build authz MsgGrant")
	s.BroadcastOK("cosmos-authz-granter", grant)

	grants, err := q.Grants(s.Ctx, &authz.QueryGrantsRequest{
		Granter:    granter.String(),
		Grantee:    grantee.String(),
		MsgTypeUrl: msgType,
	})
	require.NoError(s.T, err, "authz Grants")
	require.Len(s.T, grants.Grants, 1, "expected one bank send grant")
	granterGrants, err := q.GranterGrants(s.Ctx, &authz.QueryGranterGrantsRequest{
		Granter:    granter.String(),
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "authz GranterGrants")
	require.NotEmpty(s.T, granterGrants.Grants, "granter should have grants")
	granteeGrants, err := q.GranteeGrants(s.Ctx, &authz.QueryGranteeGrantsRequest{
		Grantee:    grantee.String(),
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "authz GranteeGrants")
	require.NotEmpty(s.T, granteeGrants.Grants, "grantee should have grants")

	exec := authz.NewMsgExec(grantee, []sdk.Msg{
		banktypes.NewMsgSend(granter, receiver, cosmosCoins(s.Env.BondDenom, 1_000_000)),
	})
	s.BroadcastOK("cosmos-authz-grantee", &exec)

	tooMuch := authz.NewMsgExec(grantee, []sdk.Msg{
		banktypes.NewMsgSend(granter, receiver, cosmosCoins(s.Env.BondDenom, 100_000_000)),
	})
	s.BroadcastExpectErr("cosmos-authz-grantee", &tooMuch)

	revoke := authz.NewMsgRevoke(granter, grantee, msgType)
	s.BroadcastOK("cosmos-authz-granter", &revoke)
	_, err = q.Grants(s.Ctx, &authz.QueryGrantsRequest{
		Granter:    granter.String(),
		Grantee:    grantee.String(),
		MsgTypeUrl: msgType,
	})
	require.Error(s.T, err, "authz Grants should be not-found after revoke")

	missing := authz.NewMsgRevoke(granter, grantee, msgType)
	s.BroadcastExpectErr("cosmos-authz-granter", &missing)
	s.logf("cosmos authz complete (grant, exec, revoke, overspend/missing-grant)")
}
