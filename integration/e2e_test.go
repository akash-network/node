// +build integration,!mainnet

package integration

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankcli "github.com/cosmos/cosmos-sdk/x/bank/client/testutil"

	ptestutil "github.com/ovrclk/akash/provider/testutil"
	"github.com/ovrclk/akash/testutil"
	deploycli "github.com/ovrclk/akash/x/deployment/client/cli"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/ovrclk/akash/x/provider/client/cli"
	"github.com/ovrclk/akash/x/provider/types"
)

// IntegrationTestSuite wraps testing components
type IntegrationTestSuite struct {
	suite.Suite

	cfg     network.Config
	network *network.Network
}

func (s *IntegrationTestSuite) SetupSuite() {
	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1
	cfg.MinGasPrices = ""
	cfg.BondDenom = "stake"

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	kb := s.network.Validators[0].ClientCtx.Keyring
	_, _, err := kb.NewMnemonic("keyBar", keyring.English, sdk.FullFundraiserPath, hd.Secp256k1)
	s.Require().NoError(err)
	_, _, err = kb.NewMnemonic("keyFoo", keyring.English, sdk.FullFundraiserPath, hd.Secp256k1)
	s.Require().NoError(err)

	_, err = s.network.WaitForHeight(1)
	s.Require().NoError(err)
}

func (s *IntegrationTestSuite) TearDownSuite() {
	val := s.network.Validators[0]
	keyTenant, err := val.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)
	resp, err := deploycli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	deployResp := &dtypes.QueryDeploymentsResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1, "Deployment Create Failed")
	deployments := deployResp.Deployments

	// get queried deployment
	createdDep := deployments[0]
	// teardown lease
	_, err = deploycli.TxCloseDeploymentExec(
		val.ClientCtx,
		keyTenant.GetAddress(),
		fmt.Sprintf("--owner=%s", createdDep.Groups[0].GroupID.Owner),
		fmt.Sprintf("--dseq=%v", createdDep.Deployment.DeploymentID.DSeq),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(3))

	// test query deployments with state filter closed
	resp, err = deploycli.QueryDeploymentsExec(
		val.ClientCtx.WithOutputFormat("json"),
		"--state=closed",
	)
	s.Require().NoError(err)

	qResp := &dtypes.QueryDeploymentsResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), qResp)
	s.Require().NoError(err)
	s.Require().Len(qResp.Deployments, 1, "Deployment Close Failed")

	s.network.Cleanup()
}

func (s *IntegrationTestSuite) TestE2EApp() {
	val := s.network.Validators[0]

	// Send coins value
	sendTokens := sdk.NewInt64Coin(s.cfg.BondDenom, 1000)

	// Setup a Provider key
	keyProvider, err := val.ClientCtx.Keyring.Key("keyFoo")
	s.Require().NoError(err)

	// give provider some coins
	_, err = bankcli.MsgSendExec(
		val.ClientCtx,
		val.Address,
		keyProvider.GetAddress(),
		sdk.NewCoins(sendTokens),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// Set up second tenant key
	keyTenant, err := val.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)

	// give tenant some coins too
	_, err = bankcli.MsgSendExec(
		val.ClientCtx,
		val.Address,
		keyTenant.GetAddress(),
		sdk.NewCoins(sendTokens),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// address for provider to listen on
	_, port, err := server.FreeTCPAddr()
	require.NoError(s.T(), err)
	provHost := fmt.Sprintf("localhost:%s", port)
	provURL := url.URL{
		Host:   provHost,
		Scheme: "http",
	}
	provFileStr := fmt.Sprintf(providerTemplate, provURL.String())
	tmpFile, err := ioutil.TempFile(s.network.BaseDir, "provider.yaml")
	require.NoError(s.T(), err)

	_, err = tmpFile.WriteString(provFileStr)
	require.NoError(s.T(), err)

	defer func() {
		err := tmpFile.Close()
		require.NoError(s.T(), err)
	}()

	fstat, err := tmpFile.Stat()
	require.NoError(s.T(), err)

	// create Provider blockchain declaration
	_, err = cli.TxCreateProviderExec(
		val.ClientCtx,
		keyProvider.GetAddress(),
		fmt.Sprintf("%s/%s", s.network.BaseDir, fstat.Name()),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	localCtx := val.ClientCtx.WithOutputFormat("json")
	// test query providers
	resp, err := cli.QueryProvidersExec(localCtx)
	s.Require().NoError(err)

	out := &types.QueryProvidersResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(keyProvider.GetAddress().String(), providers[0].Owner)

	// test query provider
	createdProvider := providers[0]
	resp, err = cli.QueryProviderExec(localCtx, createdProvider.Owner)
	s.Require().NoError(err)

	var provider types.Provider
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &provider)
	s.Require().NoError(err)
	s.Require().Equal(createdProvider, provider)

	// Run Provider service
	keyName := keyProvider.GetName()

	// Change the akash home directory for CLI to access the test keyring
	cliHome := strings.Replace(val.ClientCtx.HomeDir, "simd", "simcli", 1)

	cctx := val.ClientCtx
	go func() {
		_, err := ptestutil.RunLocalProvider(cctx,
			cctx.ChainID,
			val.RPCAddress,
			cliHome,
			keyName,
			provURL.Host,
			fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		)
		s.Require().NoError(err)
	}()

	s.Require().NoError(s.network.WaitForNextBlock())

	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	// Create Deployments and assert query to assert
	tenantAddr := keyTenant.GetAddress().String()
	_, err = deploycli.TxCreateDeploymentExec(
		val.ClientCtx,
		keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// Test query deployments ---------------------------------------------
	resp, err = deploycli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	deployResp := &dtypes.QueryDeploymentsResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1, "Deployment Create Failed")
	deployments := deployResp.Deployments
	s.Require().Equal(tenantAddr, deployments[0].Deployment.DeploymentID.Owner)

	// test query deployment
	createdDep := deployments[0]
	resp, err = deploycli.QueryDeploymentExec(val.ClientCtx.WithOutputFormat("json"), createdDep.Deployment.DeploymentID)
	s.Require().NoError(err)

	deploymentResp := dtypes.DeploymentResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &deploymentResp)
	s.Require().NoError(err)
	s.Require().Equal(createdDep, deploymentResp)
	s.Require().NotEmpty(deploymentResp.Deployment.Version)

	// test query deployments with filters -----------------------------------
	resp, err = deploycli.QueryDeploymentsExec(
		val.ClientCtx.WithOutputFormat("json"),
		fmt.Sprintf("--owner=%s", tenantAddr),
		fmt.Sprintf("--dseq=%v", createdDep.Deployment.DeploymentID.DSeq),
	)
	s.Require().NoError(err, "Error when fetching deployments with owner filter")

	deployResp = &dtypes.QueryDeploymentsResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1)

	// Assert orders created by provider
	// test query orders
	resp, err = mcli.QueryOrdersExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	result := &mtypes.QueryOrdersResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	orders := result.Orders
	s.Require().Equal(tenantAddr, orders[0].OrderID.Owner)

	// Wait for then EndBlock to handle bidding and creating lease
	s.Require().NoError(s.waitForBlocksCommitted(6))

	// Assert provider made bid and created lease; test query leases ---------
	resp, err = mcli.QueryLeasesExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)
	lease := leaseRes.Leases[0]
	lid := lease.LeaseID
	s.Require().Equal(keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider ----------------------------------------------
	bID := mtypes.BidID{
		Provider: lid.Provider,
		Owner:    lid.Owner,
		DSeq:     lid.DSeq,
		GSeq:     lid.GSeq,
		OSeq:     lid.OSeq,
	}

	_, err = ptestutil.TestSendManifest(val.ClientCtx.WithOutputFormat("json"), bID, deploymentPath)
	s.Require().NoError(err)

	s.Require().NoError(s.waitForBlocksCommitted(20))

	host, appPort := appEnv(s.T())
	appURL := fmt.Sprintf("http://%s:%s/", host, appPort)
	queryApp(s.T(), appURL, 50)
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) waitForBlocksCommitted(height int) error {
	h, err := s.network.LatestHeight()
	if err != nil {
		return err
	}

	blocksToWait := h + int64(height)
	_, err = s.network.WaitForHeightWithTimeout(blocksToWait, time.Duration(blocksToWait+1)*5*time.Second)
	if err != nil {
		return err
	}

	return nil
}

// TestQueryApp enables rapid testing of the querying functionality locally
// Not for CI tests.
func TestQueryApp(t *testing.T) {
	host, appPort := appEnv(t)

	appURL := fmt.Sprintf("http://%s:%s/", host, appPort)
	queryApp(t, appURL, 1)
}
