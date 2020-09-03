// +build integration,!mainnet

package integrationprotobuf

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/provider/cmd"
	"github.com/ovrclk/akash/testutil"
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
	cfg.NumValidators = 5  // To enable using multiple keys assigned to validators
	cfg.CleanupDir = false // TODO: remove until debugging is complete

	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	_, err := s.network.WaitForHeight(1)
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
	s.T().Logf("%#v", val)

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

	localCtx := val.ClientCtx.WithOutputFormat("json")
	cctx := val.ClientCtx

	// test query providers
	resp, err := cli.QueryProvidersExec(localCtx)
	s.Require().NoError(err)

	var out *types.QueryProvidersResponse = &types.QueryProvidersResponse{}
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
	s.T().Logf("%s", resp.Bytes())
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(val.Address.String(), providers[0].Owner.String())

	// test query provider
	createdProvider := providers[0]
	resp, err = cli.QueryProviderExec(localCtx, createdProvider.Owner)
	s.Require().NoError(err)

	var provider types.Provider
	err = val.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &provider)
	s.Require().NoError(err)
	s.Require().Equal(createdProvider, provider)

	var keyName string
	s.T().Logf("%#v", provider)
	s.T().Logf("--from=%q", cctx.From)
	keyInfo, err := cctx.Keyring.List()
	s.Require().NoError(err)
	for _, k := range keyInfo {
		s.T().Logf("key: %#v", k)
		if k.GetName() != "" {
			s.T().Logf("setting key Name: %q", k.GetName())
			keyName = k.GetName()
		}
	}
	cliHome := strings.Replace(val.ClientCtx.HomeDir, "simd", "simcli", 1)

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
	}()

	//go RunLocalProvider(ctx, )
	//provider... --from=%s --cluster-k8s --gateway-listen-address=%s %s %s"
	/*
		// Run provider service
		provProc, provHost := f.ProviderStart(keyFoo, provHost)
		defer func() { // shutdown provider
			err := provProc.Stop(true)
			require.NoError(t, err)
			so, se, err := provProc.ReadAll()
			require.NoError(t, err)
			assert.Empty(t, se, "provider output", so)
		}()
	*/
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
// Assert provider launches app in kind cluster
func queryApp(t *testing.T, appURL string) {
	req, err := http.NewRequest("GET", appURL, nil)
	require.NoError(t, err)
	req.Host = "hello.localhost" // NOTE: cannot be inserted as a req.Header element, that is overwritten by this req.Host field.
	req.Header.Add("Cache-Control", "no-cache")
	req.Header.Add("Connection", "keep-alive")
	// Assert that the service is accessible. Unforetunately brittle single request.
	tr := &http.Transport{
		DisableKeepAlives: false,
	}
	client := &http.Client{
		Transport: tr,
	}

	// retry mechanism
	var resp *http.Response
	for i := 0; i < 50; i++ {
		time.Sleep(1 * time.Second) // reduce absurdly long wait period
		resp, err = client.Do(req)
		if err != nil {
			t.Log(err)
			continue
		}
		if resp != nil && resp.StatusCode == http.StatusOK {
			err = nil
			break
		}
	}
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	bytes, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bytes), "The Future of The Cloud is Decentralized")
}

// appEnv asserts that there is an addressable docker container for KinD
func appEnv(t *testing.T) (string, string) {
	host := os.Getenv("KIND_APP_IP")
	require.NotEmpty(t, host)
	appPort := os.Getenv("KIND_APP_PORT")
	require.NotEmpty(t, appPort)
	return host, appPort
}
