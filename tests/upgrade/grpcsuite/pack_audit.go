package grpcsuite

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	av1 "pkg.akt.dev/go/node/audit/v1"
	tattr "pkg.akt.dev/go/node/types/attributes/v1"
)

type auditPack struct{}

func (auditPack) Name() string { return "audit" }

func (auditPack) Available(d *Discovery) bool { return d.HasModule("akash.audit.v1") }

func (auditPack) Run(s *Suite) {
	providerSigner, ok := s.World.Get(wProviderSigner).(string)
	require.True(s.T, ok, "audit pack needs the provider pack")
	require.NotEmpty(s.T, providerSigner, "audit pack needs the provider signer")
	provider := s.Addr(providerSigner)
	auditor := s.FundAccountDefault("auditor")

	attrs := tattr.Attributes{
		{Key: "region", Value: "us-west"},
		{Key: "audited", Value: "true"},
	}

	// Auditor attests the provider's attributes.
	s.BroadcastOK("auditor", &av1.MsgSignProviderAttributes{
		Owner:      provider.String(),
		Auditor:    auditor.String(),
		Attributes: attrs,
	})

	// All four audit queries run against the freshly-signed attributes, BEFORE the
	// delete below removes them.
	q := av1.NewQueryClient(s.Conn)

	all, err := q.AllProvidersAttributes(s.Ctx, &av1.QueryAllProvidersAttributesRequest{Pagination: &sdkquery.PageRequest{Limit: 100}})
	require.NoError(s.T, err, "AllProvidersAttributes")
	require.NotEmpty(s.T, all.Providers, "expected at least one audited provider")

	pa, err := q.ProviderAttributes(s.Ctx, &av1.QueryProviderAttributesRequest{Owner: provider.String(), Pagination: &sdkquery.PageRequest{Limit: 50}})
	require.NoError(s.T, err, "ProviderAttributes")
	require.NotEmpty(s.T, pa.Providers, "provider should have audited attributes")
	require.Equal(s.T, provider.String(), pa.Providers[0].Owner, "ProviderAttributes should be scoped to the provider")

	pca, err := q.ProviderAuditorAttributes(s.Ctx, &av1.QueryProviderAuditorRequest{Owner: provider.String(), Auditor: auditor.String()})
	require.NoError(s.T, err, "ProviderAuditorAttributes")
	require.NotEmpty(s.T, pca.Providers, "auditor should have attested the provider")

	aa, err := q.AuditorAttributes(s.Ctx, &av1.QueryAuditorAttributesRequest{Auditor: auditor.String(), Pagination: &sdkquery.PageRequest{Limit: 50}})
	require.NoError(s.T, err, "AuditorAttributes")
	require.NotEmpty(s.T, aa.Providers, "auditor should have audited providers")

	auditNegatives(s, providerSigner, auditor)

	// Auditor revokes the attested attributes (kept last so the queries above see them).
	s.BroadcastOK("auditor", &av1.MsgDeleteProviderAttributes{
		Owner:   provider.String(),
		Auditor: auditor.String(),
		Keys:    []string{"region", "audited"},
	})
	s.logf("audit complete (sign, query all 4, delete provider attributes)")
}

func auditNegatives(s *Suite, providerSigner string, auditor sdk.AccAddress) {
	s.T.Helper()
	provider := s.Addr(providerSigner).String()

	// The Auditor field is the required signer. A tx signed by "auditor" but
	// declaring a DIFFERENT auditor must be rejected (signer mismatch). This holds
	// for both sign and delete.
	other := s.FundAccountDefault("auditor2")
	s.BroadcastExpectErr("auditor", &av1.MsgSignProviderAttributes{
		Owner:      provider,
		Auditor:    other.String(),
		Attributes: tattr.Attributes{{Key: "region", Value: "us-west"}},
	})
	s.BroadcastExpectErr("auditor", &av1.MsgDeleteProviderAttributes{
		Owner:   provider,
		Auditor: other.String(),
		Keys:    []string{"region"},
	})
}
