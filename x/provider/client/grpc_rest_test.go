package client_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkrest "github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/provider/client/cli"
	"github.com/ovrclk/akash/x/provider/types"
)

type IntegrationTestSuite struct {
	suite.Suite

	cfg      network.Config
	network  *network.Network
	provider types.Provider
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	_, err := s.network.WaitForHeight(1)
	s.Require().NoError(err)

	val := s.network.Validators[0]

	providerPath, err := filepath.Abs("./../testdata/provider.yaml")
	s.Require().NoError(err)

	// create deployment
	_, err = cli.TxCreateProviderExec(
		val.ClientCtx,
		val.Address,
		providerPath,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// get provider
	resp, err := cli.QueryProvidersExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &types.QueryProvidersResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(val.Address.String(), providers[0].Owner)

	s.provider = providers[0]
}

func (s *IntegrationTestSuite) TestGetProviders() {
	val := s.network.Validators[0]
	provider := s.provider

	testCases := []struct {
		name    string
		url     string
		expResp types.Provider
		expLen  int
	}{
		{
			"get providers without pagination",
			fmt.Sprintf("%s/akash/provider/v1beta1/providers", val.APIAddress),
			provider,
			1,
		},
		{
			"get providers with pagination",
			fmt.Sprintf("%s/akash/provider/v1beta1/providers?pagination.offset=2", val.APIAddress),
			types.Provider{},
			0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var providers types.QueryProvidersResponse
			err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp, &providers)

			s.Require().NoError(err)
			s.Require().Len(providers.Providers, tc.expLen)
			if tc.expLen != 0 {
				s.Require().Equal(tc.expResp, providers.Providers[0])
			}
		})
	}
}

func (s *IntegrationTestSuite) TestGetProvider() {
	val := s.network.Validators[0]
	provider := s.provider

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp types.Provider
	}{
		{
			"get group with empty input",
			fmt.Sprintf("%s/akash/provider/v1beta1/providers/%s", val.APIAddress, ""),
			true,
			types.Provider{},
		},
		{
			"get provider with invalid input",
			fmt.Sprintf("%s/akash/provider/v1beta1/providers/%s", val.APIAddress, "hellohai"),
			true,
			types.Provider{},
		},
		{
			"valid get provider request",
			fmt.Sprintf("%s/akash/provider/v1beta1/providers/%s", val.APIAddress, provider.Owner),
			false,
			provider,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdkrest.GetRequest(tc.url)
			s.Require().NoError(err)

			var out types.QueryProviderResponse
			err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Provider)
			}
		})
	}
}

func (s *IntegrationTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	s.network.Cleanup()
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
