package cli_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/go/cli"

	"pkg.akt.dev/akashd/testutil"
	"pkg.akt.dev/akashd/testutil/network"
	pcli "pkg.akt.dev/akashd/x/provider/client/cli"
)

type IntegrationTestSuite struct {
	suite.Suite

	cfg     network.Config
	network *network.Network
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.T().Log("setting up integration test suite")

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	_, err := s.network.WaitForHeight(1)
	s.Require().NoError(err)
}

func (s *IntegrationTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	s.network.Cleanup()
}

func (s *IntegrationTestSuite) TestProvider() {
	val := s.network.Validators[0]

	providerPath, err := filepath.Abs("../../testdata/provider.yaml")
	s.Require().NoError(err)

	providerPath2, err := filepath.Abs("../../testdata/provider2.yaml")
	s.Require().NoError(err)

	// create deployment
	_, err = pcli.TxCreateProviderExec(
		val.ClientCtx,
		val.Address,
		providerPath,
		fmt.Sprintf("--%s=true", cli.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", cli.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", cli.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", cli.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	// test query providers
	resp, err := pcli.QueryProvidersExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	out := &types.QueryProvidersResponse{}
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(val.Address.String(), providers[0].Owner)

	// test query provider
	createdProvider := providers[0]
	resp, err = pcli.QueryProviderExec(val.ClientCtx.WithOutputFormat("json"), createdProvider.Owner)
	s.Require().NoError(err)

	var provider types.Provider
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &provider)
	s.Require().NoError(err)
	s.Require().Equal(createdProvider, provider)

	// test updating provider
	_, err = pcli.TxUpdateProviderExec(
		val.ClientCtx,
		val.Address,
		providerPath2,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, cli.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	resp, err = pcli.QueryProviderExec(val.ClientCtx.WithOutputFormat("json"), createdProvider.Owner)
	s.Require().NoError(err)

	var providerV2 types.Provider
	err = val.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &providerV2)
	s.Require().NoError(err)
	s.Require().NotEqual(provider.HostURI, providerV2.HostURI)
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
