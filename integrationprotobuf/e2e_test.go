// +build integration,!mainnet

package integrationprotobuf

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankcli "github.com/cosmos/cosmos-sdk/x/bank/client/testutil"

	"github.com/ovrclk/akash/provider/cmd"
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
	s.T().Log("setting up integration test suite")

	/*
		var err error
		s.tmpDir, err = ioutil.TempDir("", "akash_integration_"+s.T().Name()+"_")
		require.NoError(s.T(), err)

		// Prevent akash errors on exit due to data saving behavior.
		tmpStat, err := os.Lstat(s.tmpDir)
		require.NoError(s.T(), err)
		err = os.MkdirAll(fmt.Sprintf("%s/.akashd/data/cs.wal", s.tmpDir), tmpStat.Mode())
		require.NoError(s.T(), err)

				servAddr, port, err := server.FreeTCPAddr()
				require.NoError(s.T(), err)

				p2pAddr, _, err := server.FreeTCPAddr()
				require.NoError(s.T(), err)
			buildDir := os.Getenv("BUILDDIR")
			if buildDir == "" {
				buildDir, err = filepath.Abs("../_build/")
				require.NoError(s.T(), err)
			}
	*/

	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1  // To enable using multiple keys assigned to validators
	cfg.CleanupDir = false // TODO: remove until debugging is complete
	//akashBondDenom := "akash"
	//cfg.BondDenom = akashBondDenom
	//cfg.MinGasPrices = fmt.Sprintf("0.000006%s", akashBondDenom)

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

/*
func (s *IntegrationTestSuite) TestNetwork_Liveness() {
	h, err := s.network.WaitForHeightWithTimeout(5, time.Minute)
	s.Require().NoError(err, "expected to reach 5 blocks; got %d", h)
}
*/

func (s *IntegrationTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	s.network.Cleanup()
}

func (s *IntegrationTestSuite) TestE2EApp() {
	val := s.network.Validators[0]

	// Send coins value
	sendTokens := sdk.NewInt64Coin(s.cfg.BondDenom, 1000)

	// Setup a Provider key
	keyFoo, err := val.ClientCtx.Keyring.Key("keyFoo")
	s.Require().NoError(err)
	_, err = bankcli.MsgSendExec(
		val.ClientCtx,
		val.Address,
		keyFoo.GetAddress(),
		sdk.NewCoins(sendTokens),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// Set up second tenant key
	keyBar, err := val.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)
	// Send coins from validator to keyBar
	_, err = bankcli.MsgSendExec(
		val.ClientCtx,
		val.Address,
		keyBar.GetAddress(),
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
		err = tmpFile.Close()
		require.NoError(s.T(), err)
	}()
	fstat, err := tmpFile.Stat()
	require.NoError(s.T(), err)
	providerPath := fmt.Sprintf("%s/%s", s.network.BaseDir, fstat.Name())
	s.T().Logf("tfilePath: %q", providerPath)

	// create Provider blockchain declaration
	_, err = cli.TxCreateProviderExec(
		val.ClientCtx,
		keyFoo.GetAddress(),
		providerPath,
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

	var out *types.QueryProvidersResponse = &types.QueryProvidersResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
	s.T().Logf("%s", resp.Bytes())
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(keyFoo.GetAddress().String(), providers[0].Owner.String())

	// test query provider
	createdProvider := providers[0]
	resp, err = cli.QueryProviderExec(localCtx, createdProvider.Owner)
	s.Require().NoError(err)

	var provider types.Provider
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &provider)
	s.Require().NoError(err)
	s.Require().Equal(createdProvider, provider)

	var keyName string
	keyName = keyFoo.GetName()

	// Change the akash home directory for CLI to access the test keyring
	cliHome := strings.Replace(val.ClientCtx.HomeDir, "simd", "simcli", 1)
	// Launch the provider service in goroutine
	cctx := val.ClientCtx
	go func() {
		buf, err := cmd.RunLocalProvider(cctx,
			cctx.ChainID,
			val.RPCAddress,
			cliHome,
			keyName,
			provURL.Host,
		)
		s.T().Log(buf.String())
		s.Require().NoError(err)
		// TODO: Kill mechanism on cleanup
	}()
	s.Require().NoError(s.network.WaitForNextBlock())

	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	tenantAddr := keyBar.GetAddress().String()
	s.T().Logf("%#v", tenantAddr)
	buf, err := deploycli.TxCreateDeploymentExec(
		val.ClientCtx,
		keyBar.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.T().Log(buf.String())
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// test query deployments
	resp, err = deploycli.QueryDeploymentsExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	// Create Deployments and assert query to assert
	var deployResp *dtypes.QueryDeploymentsResponse = &dtypes.QueryDeploymentsResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), deployResp)
	s.T().Log(deployResp.String())
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1, "Deployment Create Failed")
	deployments := deployResp.Deployments
	s.Require().Equal(tenantAddr, deployments[0].Deployment.DeploymentID.Owner.String())

	// test query deployment
	createdDep := deployments[0]
	resp, err = deploycli.QueryDeploymentExec(val.ClientCtx.WithOutputFormat("json"), createdDep.Deployment.DeploymentID)
	s.Require().NoError(err)

	var deployment dtypes.DeploymentResponse
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &deployment)
	s.Require().NoError(err)
	s.Require().Equal(createdDep, deployment)

	// test query deployments with filters
	resp, err = deploycli.QueryDeploymentsExec(
		val.ClientCtx.WithOutputFormat("json"),
		fmt.Sprintf("--owner=%s", tenantAddr), // use valTenant address
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

	var result *mtypes.QueryOrdersResponse = &mtypes.QueryOrdersResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	orders := result.Orders
	s.Require().Equal(tenantAddr, orders[0].OrderID.Owner.String())

	// Wait for then EndBlock to handle bidding and creating lease
	h, err := s.network.LatestHeight()
	s.Require().NoError(err)
	s.network.WaitForHeight(h + int64(3))

	// Assert provider made bid and created lease; test query leases
	resp, err = mcli.QueryLeasesExec(val.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	var leaseRes *mtypes.QueryLeasesResponse = &mtypes.QueryLeasesResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)
	leases := leaseRes.Leases
	//s.Require().Equal(keyBar.GetAddress().String(), leases[0].LeaseID.Provider.String())
	s.Assert().Len(leases, 1)

	// TODO: Send manifest
	// akashctl provider send-manifest --owner akash1c09qqu9jp658jfuc0wa5wxhnsv99jwzvlv63u6 --dseq 7 --gseq 1 --oseq 1 --provider akash1p5a59r458sx6rt74ttku2zh0pdpsj5xtvvfzpw ./../_run/kube/deployment.yaml --home=/tmp/akash_integration_TestE2EApp_324892307/.akashctl --node=tcp://0.0.0.0:41863

	// TODO: Assert that service is running

}

func TestIntegrationTestSuite(t *testing.T) {
	host, appPort := appEnv(t)
	t.Logf("kind: %s:%s", host, appPort)
	suite.Run(t, new(IntegrationTestSuite))
}

/*
func TestE2EApp(t *testing.T) {
	host, appPort := appEnv(t)

	f := InitFixtures(t)
	defer f.Cleanup() // NOTE: defer statement ordering matters.

	cfg := network.DefaultConfig()
	cfg.NumValidators = 1



	// start akashd server
	proc := f.AkashdStart()
	defer func() { // shutdown akashd
		err := proc.Stop(false)
		require.NoError(t, err)
		so, se, err := proc.ReadAll()
		require.NoError(t, err)
		assert.Empty(t, se, "akashd output", so)
	}()

			// Save key addresses for later use
			fooAddr := f.KeyAddress(keyFoo)
			barAddr := f.KeyAddress(keyBar)

			fooAcc := f.QueryAccount(fooAddr)
			startTokens := sdk.TokensFromConsensusPower(denomStartValue)
			require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

			// address for provider to listen on
			_, port, err := server.FreeTCPAddr()
			require.NoError(t, err)
			provHost := fmt.Sprintf("localhost:%s", port)
			provURL := url.URL{
				Host:   provHost,
				Scheme: "http",
			}

			provFileStr := fmt.Sprintf(providerTemplate, provURL.String())
			tmpFile, err := ioutil.TempFile(f.RootDir, "provider.yaml")
			require.NoError(t, err)

			_, err = tmpFile.WriteString(provFileStr)
			require.NoError(t, err)
			defer func() {
				err = tmpFile.Close()
				require.NoError(t, err)
			}()
			fstat, err := tmpFile.Stat()
			require.NoError(t, err)
			tfilePath := fmt.Sprintf("%s/%s", f.RootDir, fstat.Name())

			// Create provider
			f.TxCreateProviderFromFile(tfilePath, fmt.Sprintf("--from=%s", keyFoo), "-y")
			cosmostests.WaitForNextNBlocksTM(1, f.Port)

			// test query providers
			providers := f.QueryProviders()
			require.Len(t, providers, 1, "Creating provider failed")
			require.Equal(t, fooAddr.String(), providers[0].Owner.String())

			// test query provider
			createdProvider := providers[0]
			provider := f.QueryProvider(createdProvider.Owner.String())
			require.Equal(t, createdProvider, provider)

			// Run provider service
			provProc, provHost := f.ProviderStart(keyFoo, provHost)
			defer func() { // shutdown provider
				err := provProc.Stop(true)
				require.NoError(t, err)
				so, se, err := provProc.ReadAll()
				require.NoError(t, err)
				assert.Empty(t, se, "provider output", so)
			}()

			// Apply SDL to provider
			// Create deployment for `keyBar`
			f.TxCreateDeployment(deploymentOvrclkApp, fmt.Sprintf("--from=%s", keyBar), "-y")
			cosmostests.WaitForNextNBlocksTM(1, f.Port)

			// test query deployments
			deployments, err := f.QueryDeployments()
			require.NoError(t, err)
			require.Len(t, deployments, 1, "Deployment Create Failed")
			require.Equal(t, barAddr.String(), deployments[0].Deployment.DeploymentID.Owner.String())

			// test query deployment
			createdDep := deployments[0]
			deployment := f.QueryDeployment(createdDep.Deployment.DeploymentID)
			require.Equal(t, createdDep, deployment)

			cosmostests.WaitForNextNBlocksTM(1, f.Port)



			orders, err := f.QueryOrders()
			require.NoError(t, err)
			require.Len(t, orders, 1, "Order creation failed")
			require.Equal(t, barAddr.String(), orders[0].OrderID.Owner.String())

			// Assert that there are no leases yet
			leases, err := f.QueryLeases()
			require.NoError(t, err)
			require.Len(t, leases, 0, "no Leases should be created yet")

			// Wait for then EndBlock to handle bidding and creating lease
			cosmostests.WaitForNextNBlocksTM(5, f.Port)

			// Assert provider made bid
			leases, err = f.QueryLeases()
			require.NoError(t, err)
			require.Len(t, leases, 1, "Lease should be created after bidding completes")
			lease := leases[0]

			// Provide manifest to provider service
			f.SendManifest(lease, deploymentOvrclkApp)

			// Wait for App to deploy
			cosmostests.WaitForNextNBlocksTM(5, f.Port)

			// Assert provider launches app in kind
			appURL := fmt.Sprintf("http://%s:%s/", host, appPort)
			queryApp(t, appURL)

			// Close deployment to clean up container
			//  Teardown/cleanup
			// TODO: uncomment
			//f.TxCloseDeployment(fmt.Sprintf("--from=%s --dseq=%v", keyBar, createdDep.Deployment.DeploymentID.DSeq), "-y")
			//tests.WaitForNextNBlocksTM(3, f.Port)
		}

		func TestQueryApp(t *testing.T) {
			host, appPort := appEnv(t)

			appURL := fmt.Sprintf("http://%s:%s/", host, appPort)
			queryApp(t, appURL)
		}

*/
