package grpcsuite

import (
	"encoding/pem"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	cv1 "pkg.akt.dev/go/node/cert/v1"
	certutils "pkg.akt.dev/go/node/cert/v1/utils"
)

type certPack struct{}

func (certPack) Name() string { return "cert" }

func (certPack) Available(d *Discovery) bool { return d.HasModule("akash.cert.v1") }

func (certPack) Run(s *Suite) {
	owner := s.FundAccountDefault("certowner")
	certPEM, pubPEM := certGenerate(s, owner)

	s.BroadcastOK("certowner", &cv1.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   certPEM,
		Pubkey: pubPEM,
	})

	q := cv1.NewQueryClient(s.Conn)

	// Certificates filtered by owner (with pagination); assert the new cert is
	// present and valid.
	resp, err := q.Certificates(s.Ctx, &cv1.QueryCertificatesRequest{
		Filter:     cv1.CertificateFilter{Owner: owner.String()},
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "Certificates by owner")
	require.NotEmpty(s.T, resp.Certificates, "owner should have a certificate")
	serial := resp.Certificates[0].Serial
	require.Equal(s.T, cv1.CertificateValid, resp.Certificates[0].Certificate.State, "new cert should be valid")

	// Certificates filtered by owner+serial; assert exactly one.
	one, err := q.Certificates(s.Ctx, &cv1.QueryCertificatesRequest{
		Filter: cv1.CertificateFilter{Owner: owner.String(), Serial: serial},
	})
	require.NoError(s.T, err, "Certificates by owner+serial")
	require.Len(s.T, one.Certificates, 1, "owner+serial should return exactly one cert")

	certNegatives(s, owner, serial)

	// Revoke (kept last) and confirm the state transitions to revoked.
	s.BroadcastOK("certowner", &cv1.MsgRevokeCertificate{ID: cv1.ID{Owner: owner.String(), Serial: serial}})
	rev, err := q.Certificates(s.Ctx, &cv1.QueryCertificatesRequest{
		Filter: cv1.CertificateFilter{Owner: owner.String(), Serial: serial},
	})
	require.NoError(s.T, err, "Certificates after revoke")
	require.NotEmpty(s.T, rev.Certificates)
	require.Equal(s.T, cv1.CertificateRevoked, rev.Certificates[0].Certificate.State, "cert should be revoked")
	s.logf("cert lifecycle complete (create + revoke, serial %s)", serial)
}

// certGenerate produces a PEM cert + pubkey for owner via KeyPairManager.
func certGenerate(s *Suite, owner sdk.AccAddress) (certPEM, pubPEM []byte) {
	s.T.Helper()
	cctx := s.Cctx.WithHomeDir(s.T.TempDir())
	kpm, err := certutils.NewKeyPairManager(cctx, owner)
	require.NoError(s.T, err, "NewKeyPairManager")
	require.NoError(s.T, kpm.Generate(time.Now().Add(-time.Hour), time.Now().Add(365*24*time.Hour), nil), "generate cert")
	certDER, _, pubDER, err := kpm.Read()
	require.NoError(s.T, err, "read cert")
	return pem.EncodeToMemory(&pem.Block{Type: cv1.PemBlkTypeCertificate, Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: cv1.PemBlkTypeECPublicKey, Bytes: pubDER})
}

func certNegatives(s *Suite, owner sdk.AccAddress, serial string) {
	s.T.Helper()
	// Malformed certificate / pubkey bytes.
	_, _ = s.BroadcastExpectErr("certowner", &cv1.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   []byte("-----BEGIN CERTIFICATE-----\nnot-a-real-cert\n-----END CERTIFICATE-----"),
		Pubkey: []byte("not-a-pubkey"),
	})
	// Revoke a serial that does not exist.
	_, _ = s.BroadcastExpectErr("certowner", &cv1.MsgRevokeCertificate{ID: cv1.ID{Owner: owner.String(), Serial: "99999999999999999999"}})
	// Revoke with an owner that does not match the signer (signed by certowner).
	other := s.FundAccountDefault("certother")
	_, _ = s.BroadcastExpectErr("certowner", &cv1.MsgRevokeCertificate{ID: cv1.ID{Owner: other.String(), Serial: serial}})
}
