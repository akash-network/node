package grpcsuite

import (
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	tattr "pkg.akt.dev/go/node/types/attributes/v1"
)

// World keys published by the provider pack for downstream packs (market, audit).
const wProviderSigner = "provider.signer" // keyring name of the registered provider

type providerPack struct{}

func (providerPack) Name() string { return "provider" }

func (providerPack) Available(d *Discovery) bool {
	return d.HasModule("akash.provider.v1beta4")
}

func (providerPack) Run(s *Suite) {
	q := ptypes.NewQueryClient(s.Conn)

	attrs := tattr.Attributes{
		{Key: "region", Value: "us-west"},
		{Key: "tier", Value: "community"},
	}
	info := ptypes.Info{EMail: "ops@grpcsuite.test", Website: "https://grpcsuite.test"}

	// Primary provider — registered and kept for the market/audit packs.
	provider := s.FundAccountDefault("provider")
	s.BroadcastOK("provider", &ptypes.MsgCreateProvider{
		Owner:      provider.String(),
		HostURI:    "https://provider.grpcsuite.test:8443",
		Attributes: attrs,
		Info:       info,
	})

	// MsgUpdateProvider — change host + attributes.
	s.BroadcastOK("provider", &ptypes.MsgUpdateProvider{
		Owner:      provider.String(),
		HostURI:    "https://provider.grpcsuite.test:8444",
		Attributes: append(attrs, tattr.Attribute{Key: "updated", Value: "true"}),
		Info:       info,
	})

	// Queries — assert the provider reflects the update, and the list contains it.
	pResp, err := q.Provider(s.Ctx, &ptypes.QueryProviderRequest{Owner: provider.String()})
	require.NoError(s.T, err, "Provider by owner")
	require.Equal(s.T, provider.String(), pResp.Provider.Owner, "queried provider owner should match")
	require.Equal(s.T, "https://provider.grpcsuite.test:8444", pResp.Provider.HostURI, "host_uri should reflect the update")
	require.True(s.T, providerHasAttribute(pResp.Provider, "updated"), "updated attribute should be present after update")

	list, err := q.Providers(s.Ctx, &ptypes.QueryProvidersRequest{Pagination: &sdkquery.PageRequest{Limit: 100}})
	require.NoError(s.T, err, "Providers")
	require.NotEmpty(s.T, list.Providers, "expected at least one provider")
	found := false
	for _, p := range list.Providers {
		if p.Owner == provider.String() {
			found = true
			break
		}
	}
	require.True(s.T, found, "Providers list should include the registered provider")
	paged, err := q.Providers(s.Ctx, &ptypes.QueryProvidersRequest{Pagination: &sdkquery.PageRequest{Limit: 1}})
	require.NoError(s.T, err, "Providers (paginated)")
	require.LessOrEqual(s.T, len(paged.Providers), 1, "pagination Limit=1 must cap results")

	s.World.Set(wProviderSigner, "provider")

	// Negative: re-registering an already-registered owner must be rejected.
	s.BroadcastExpectErr("provider", &ptypes.MsgCreateProvider{
		Owner: provider.String(), HostURI: "https://dup.grpcsuite.test:8443", Attributes: attrs, Info: info,
	})

	// Negative: updating from an account that is not a registered provider.
	ghost := s.FundAccountDefault("provider-unregistered")
	s.BroadcastExpectErr("provider-unregistered", &ptypes.MsgUpdateProvider{
		Owner: ghost.String(), HostURI: "https://ghost.grpcsuite.test:8443", Attributes: attrs, Info: info,
	})

	// MsgDeleteProvider is intentionally disabled on-chain (providers cannot be
	// removed, to avoid orphaning leases). Exercise it and assert the expected
	// rejection; the primary provider stays registered for downstream packs.
	_, err = s.TX.Broadcast("provider", &ptypes.MsgDeleteProvider{Owner: provider.String()})
	require.Error(s.T, err, "MsgDeleteProvider should be rejected (disabled on-chain)")
	s.logf("provider lifecycle complete (create/update + queries; duplicate-create, unregistered-update, delete rejected)")
}

// providerHasAttribute reports whether the provider carries an attribute with the
// given key.
func providerHasAttribute(p ptypes.Provider, key string) bool {
	for _, a := range p.Attributes {
		if a.Key == key {
			return true
		}
	}
	return false
}
