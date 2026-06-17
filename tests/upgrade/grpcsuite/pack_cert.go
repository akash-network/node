package grpcsuite

import (
	"encoding/pem"
	"time"

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

	// Generate an mTLS client certificate signed by the owner's account key.
	// KeyPairManager derives a deterministic key from a keyring signature and writes
	// the PEM bundle under cctx.HomeDir, so point it at a throwaway temp dir.
	cctx := s.Cctx.WithHomeDir(s.T.TempDir())
	kpm, err := certutils.NewKeyPairManager(cctx, owner)
	require.NoError(s.T, err, "NewKeyPairManager")
	require.NoError(s.T, kpm.Generate(time.Now().Add(-time.Hour), time.Now().Add(365*24*time.Hour), nil), "generate cert")
	// Read returns raw DER bytes; PEM-wrap them with the chain's expected block
	// types (mirrors the CLI's cert publish).
	certDER, _, pubDER, err := kpm.Read()
	require.NoError(s.T, err, "read cert")

	s.BroadcastOK("certowner", &cv1.MsgCreateCertificate{
		Owner:  owner.String(),
		Cert:   pem.EncodeToMemory(&pem.Block{Type: cv1.PemBlkTypeCertificate, Bytes: certDER}),
		Pubkey: pem.EncodeToMemory(&pem.Block{Type: cv1.PemBlkTypeECPublicKey, Bytes: pubDER}),
	})

	q := cv1.NewQueryClient(s.Conn)
	resp, err := q.Certificates(s.Ctx, &cv1.QueryCertificatesRequest{
		Filter:     cv1.CertificateFilter{Owner: owner.String()},
		Pagination: &sdkquery.PageRequest{Limit: 10},
	})
	require.NoError(s.T, err, "Certificates")
	require.NotEmpty(s.T, resp.Certificates, "owner should have a certificate")

	serial := resp.Certificates[0].Serial
	s.BroadcastOK("certowner", &cv1.MsgRevokeCertificate{ID: cv1.ID{Owner: owner.String(), Serial: serial}})
	s.logf("cert lifecycle complete (create + revoke, serial %s)", serial)
}
