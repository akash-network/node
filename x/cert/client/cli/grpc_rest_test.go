package cli_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkrest "github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/network"
	atypes "github.com/akash-network/node/types"
	ccli "github.com/akash-network/node/x/cert/client/cli"
)

type GRPCRestTestSuite struct {
	suite.Suite

	cfg     network.Config
	network *network.Network
	certs   types.CertificatesResponse
}

func (s *GRPCRestTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1

	s.cfg = cfg
	s.network = network.New(s.T(), s.cfg)

	_, err := s.network.WaitForHeight(1)
	s.Require().NoError(err)

	val := s.network.Validators[0]

	// Generate client certificate
	_, err = ccli.TxGenerateClientExec(
		context.Background(),
		val.ClientCtx,
		val.Address,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// Publish client certificate
	_, err = ccli.TxPublishClientExec(
		context.Background(),
		val.ClientCtx,
		val.Address,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// get certs
	resp, err := ccli.QueryCertificatesExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &types.QueryCertificatesResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Certificates, 1, "Certificate Create Failed")
	block, rest := pem.Decode(out.Certificates[0].Certificate.Cert)
	require.NotNil(s.T(), block)
	require.Len(s.T(), rest, 0)

	require.Equal(s.T(), block.Type, types.PemBlkTypeCertificate)

	cert, err := x509.ParseCertificate(block.Bytes)
	s.Require().NoError(err)
	s.Require().NotNil(cert)

	s.Require().Equal(val.Address.String(), cert.Issuer.CommonName)

	s.certs = out.Certificates
}

func (s *GRPCRestTestSuite) TestGetCertificates() {
	val := s.network.Validators[0]
	certs := s.certs

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp types.CertificatesResponse
		expLen  int
	}{
		{
			"get certificates without filters",
			fmt.Sprintf("%s/akash/cert/%s/certificates/list", val.APIAddress, atypes.ProtoAPIVersion),
			false,
			certs,
			1,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var certs types.QueryCertificatesResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &certs)

			if tc.expErr {
				s.Require().NotNil(err)
				s.Require().Empty(certs.Certificates)
			} else {
				s.Require().NoError(err)
				s.Require().Len(certs.Certificates, tc.expLen)
				s.Require().Equal(tc.expResp, certs.Certificates)
			}
		})
	}
}

func (s *GRPCRestTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	s.network.Cleanup()
}

func TestGRPCRestTestSuite(t *testing.T) {
	suite.Run(t, new(GRPCRestTestSuite))
}
