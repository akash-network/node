package grpcsuite

import (
	"github.com/stretchr/testify/require"

	av1 "pkg.akt.dev/go/node/audit/v1"
	tattr "pkg.akt.dev/go/node/types/attributes/v1"
)

type auditPack struct{}

func (auditPack) Name() string { return "audit" }

func (auditPack) Available(d *Discovery) bool { return d.HasModule("akash.audit.v1") }

func (auditPack) Run(s *Suite) {
	require.NotEmpty(s.T, s.World.Get(wProviderSigner), "audit pack needs the provider pack")
	provider := s.Addr("provider")
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

	q := av1.NewQueryClient(s.Conn)
	_, err := q.ProviderAttributes(s.Ctx, &av1.QueryProviderAttributesRequest{Owner: provider.String()})
	require.NoError(s.T, err, "ProviderAttributes")
	_, err = q.AuditorAttributes(s.Ctx, &av1.QueryAuditorAttributesRequest{Auditor: auditor.String()})
	require.NoError(s.T, err, "AuditorAttributes")

	// Auditor revokes the attested attributes.
	s.BroadcastOK("auditor", &av1.MsgDeleteProviderAttributes{
		Owner:   provider.String(),
		Auditor: auditor.String(),
		Keys:    []string{"region", "audited"},
	})
	s.logf("audit complete (sign + delete provider attributes)")
}
