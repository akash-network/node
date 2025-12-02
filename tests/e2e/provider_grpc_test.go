//go:build e2e.integration

package e2e

import (
	"fmt"
	"path/filepath"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"

	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"
	types "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/node/testutil"
)

type providerGRPCRestTestSuite struct {
	*testutil.NetworkTestSuite

	provider types.Provider
}

func (s *providerGRPCRestTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	// Wait for API server to be ready
	s.Require().NoError(s.Network().WaitForBlocks(2))

	providerPath, err := filepath.Abs("../../x/provider/testdata/provider.yaml")
	s.Require().NoError(err)

	ctx := s.CLIContext()

	val := s.Network().Validators[0]
	cctx := s.CLIClientContext()

	// create provider
	_, err = clitestutil.ExecTxCreateProvider(
		ctx,
		cctx,
		cli.TestFlags().
			With(providerPath).
			WithFrom(val.Address.String()).
			WithGasAuto().
			WithSkipConfirm().
			WithBroadcastModeBlock()...,
	)
	s.Require().NoError(err)

	s.Require().NoError(s.Network().WaitForBlocks(2))

	// get provider
	resp, err := clitestutil.ExecQueryProviders(
		ctx,
		cctx,
		cli.TestFlags().
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	out := &types.QueryProvidersResponse{}
	err = cctx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(val.Address.String(), providers[0].Owner)

	s.provider = providers[0]
}

func (s *providerGRPCRestTestSuite) TestGetProviders() {
	val := s.Network().Validators[0]
	cctx := s.CLIClientContext()

	provider := s.provider

	testCases := []struct {
		name    string
		url     string
		expResp types.Provider
		expLen  int
	}{
		{
			"get providers without pagination",
			fmt.Sprintf("%s/akash/provider/v1beta4/providers", val.APIAddress),
			provider,
			1,
		},
		{
			"get providers with pagination",
			fmt.Sprintf("%s/akash/provider/v1beta4/providers?pagination.offset=2", val.APIAddress),
			types.Provider{},
			0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var providers types.QueryProvidersResponse
			err = cctx.Codec.UnmarshalJSON(resp, &providers)

			s.Require().NoError(err)
			s.Require().Len(providers.Providers, tc.expLen)
			if tc.expLen != 0 {
				s.Require().Equal(tc.expResp, providers.Providers[0])
			}
		})
	}
}

func (s *providerGRPCRestTestSuite) TestGetProvider() {
	val := s.Network().Validators[0]
	cctx := s.CLIClientContext()

	provider := s.provider

	testCases := []struct {
		name    string
		url     string
		expErr  bool
		expResp types.Provider
	}{
		{
			"get provider with empty input",
			fmt.Sprintf("%s/akash/provider/v1beta4/providers/%s", val.APIAddress, ""),
			true,
			types.Provider{},
		},
		{
			"get provider with invalid input",
			fmt.Sprintf("%s/akash/provider/v1beta4/providers/%s", val.APIAddress, "hellohai"),
			true,
			types.Provider{},
		},
		{
			"valid get provider request",
			fmt.Sprintf("%s/akash/provider/v1beta4/providers/%s", val.APIAddress, provider.Owner),
			false,
			provider,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var out types.QueryProviderResponse
			err = cctx.Codec.UnmarshalJSON(resp, &out)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expResp, out.Provider)
			}
		})
	}
}
