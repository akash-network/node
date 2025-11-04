//go:build e2e.integration

package e2e

import (
	"context"
	"crypto/x509"
	"encoding/pem"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"

	"github.com/stretchr/testify/require"
	types "pkg.akt.dev/go/node/cert/v1"

	"pkg.akt.dev/node/v2/testutil"
)

type certsGRPCRestTestSuite struct {
	*testutil.NetworkTestSuite
	certs types.CertificatesResponse
}

func (s *certsGRPCRestTestSuite) TestGenerateParse() {
	ctx := context.Background()
	cctx := s.ClientContextForTest()

	addr := s.WalletForTest()

	// Generate client certificate
	_, err := clitestutil.TxGenerateClientExec(
		ctx,
		cctx,
		cli.TestFlags().
			WithFrom(addr.String())...)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// Publish client certificate
	_, err = clitestutil.TxPublishClientExec(
		ctx,
		cctx,
		cli.TestFlags().
			WithFrom(addr.String()).
			WithSkipConfirm().
			WithBroadcastModeBlock().
			WithGasAutoFlags()...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.Network().WaitForNextBlock())

	// get certs
	resp, err := clitestutil.QueryCertificatesExec(ctx, cctx, cli.TestFlags().WithOutputJSON()...)
	s.Require().NoError(err)

	out := &types.QueryCertificatesResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Certificates, 1, "Certificate Create Failed")
	block, rest := pem.Decode(out.Certificates[0].Certificate.Cert)
	require.NotNil(s.T(), block)
	require.Len(s.T(), rest, 0)

	require.Equal(s.T(), block.Type, types.PemBlkTypeCertificate)

	cert, err := x509.ParseCertificate(block.Bytes)
	s.Require().NoError(err)
	s.Require().NotNil(cert)

	s.Require().Equal(addr.String(), cert.Issuer.CommonName)

	s.certs = out.Certificates
}

// func (s *GRPCRestTestSuite) TestGetCertificates() {
// 	val := s.network.Validators[0]
// 	certs := s.certs
//
// 	testCases := []struct {
// 		name    string
// 		url     string
// 		expErr  bool
// 		expResp types.CertificatesResponse
// 		expLen  int
// 	}{
// 		{
// 			"get certificates without filters",
// 			fmt.Sprintf("%s/akash/cert/%s/certificates/list", val.APIAddress, atypes.ProtoAPIVersion),
// 			false,
// 			certs,
// 			1,
// 		},
// 	}
//
// 	for _, tc := range testCases {
// 		tc := tc
// 		s.Run(tc.name, func() {
// 			resp, err := sdkrest.GetRequest(tc.url)
// 			s.Require().NoError(err)
//
// 			var certs types.QueryCertificatesResponse
// 			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &certs)
//
// 			if tc.expErr {
// 				s.Require().NotNil(err)
// 				s.Require().Empty(certs.Certificates)
// 			} else {
// 				s.Require().NoError(err)
// 				s.Require().Len(certs.Certificates, tc.expLen)
// 				s.Require().Equal(tc.expResp, certs.Certificates)
// 			}
// 		})
// 	}
// }
