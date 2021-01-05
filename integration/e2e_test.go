// +build integration,!mainnet

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/cosmos/cosmos-sdk/server"
	bankcli "github.com/cosmos/cosmos-sdk/x/bank/client/testutil"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/x/provider/client/cli"
	"github.com/ovrclk/akash/x/provider/types"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"

	"net"
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
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	providerCmd "github.com/ovrclk/akash/provider/cmd"
	ptestutil "github.com/ovrclk/akash/provider/testutil"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	deploycli "github.com/ovrclk/akash/x/deployment/client/cli"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// IntegrationTestSuite wraps testing components
type IntegrationTestSuite struct {
	suite.Suite

	cfg         network.Config
	network     *network.Network
	validator   *network.Validator
	keyProvider keyring.Info
	keyTenant   keyring.Info
	prevLeases  mtypes.Leases

	appHost string
	appPort string
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.appHost, s.appPort = appEnv(s.T())

	// Create a network for test
	cfg := testutil.DefaultConfig()
	cfg.NumValidators = 1
	cfg.MinGasPrices = ""
	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	kb := s.network.Validators[0].ClientCtx.Keyring
	_, _, err := kb.NewMnemonic("keyBar", keyring.English, sdk.FullFundraiserPath, hd.Secp256k1)
	s.Require().NoError(err)
	_, _, err = kb.NewMnemonic("keyFoo", keyring.English, sdk.FullFundraiserPath, hd.Secp256k1)
	s.Require().NoError(err)

	// Wait for the network to start
	_, err = s.network.WaitForHeight(1)
	s.Require().NoError(err)

	//
	s.validator = s.network.Validators[0]

	// Send coins value
	sendTokens := sdk.NewInt64Coin(s.cfg.BondDenom, 9999999)

	// Setup a Provider key
	s.keyProvider, err = s.validator.ClientCtx.Keyring.Key("keyFoo")
	s.Require().NoError(err)

	// give provider some coins
	_, err = bankcli.MsgSendExec(
		s.validator.ClientCtx,
		s.validator.Address,
		s.keyProvider.GetAddress(),
		sdk.NewCoins(sendTokens),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// Set up second tenant key
	s.keyTenant, err = s.validator.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)

	// give tenant some coins too
	_, err = bankcli.MsgSendExec(
		s.validator.ClientCtx,
		s.validator.Address,
		s.keyTenant.GetAddress(),
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
		s.validator.ClientCtx,
		s.keyProvider.GetAddress(),
		fmt.Sprintf("%s/%s", s.network.BaseDir, fstat.Name()),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.network.WaitForNextBlock())

	localCtx := s.validator.ClientCtx.WithOutputFormat("json")
	// test query providers
	resp, err := cli.QueryProvidersExec(localCtx)
	s.Require().NoError(err)

	out := &types.QueryProvidersResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(s.keyProvider.GetAddress().String(), providers[0].Owner)

	// test query provider
	createdProvider := providers[0]
	resp, err = cli.QueryProviderExec(localCtx, createdProvider.Owner)
	s.Require().NoError(err)

	var provider types.Provider
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &provider)
	s.Require().NoError(err)
	s.Require().Equal(createdProvider, provider)

	// Run Provider service
	keyName := s.keyProvider.GetName()

	// Change the akash home directory for CLI to access the test keyring
	cliHome := strings.Replace(s.validator.ClientCtx.HomeDir, "simd", "simcli", 1)

	cctx := s.validator.ClientCtx
	go func() {
		_, err := ptestutil.RunLocalProvider(cctx,
			cctx.ChainID,
			s.validator.RPCAddress,
			cliHome,
			keyName,
			provURL.Host,
			fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		)
		s.Require().NoError(err)
	}()

	s.Require().NoError(s.network.WaitForNextBlock())
}

func (s *IntegrationTestSuite) TearDownSuite() {

	s.T().Log("Cleaning up after E2E tests")

	keyTenant, err := s.validator.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)
	resp, err := deploycli.QueryDeploymentsExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	deployResp := &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().False(0 == len(deployResp.Deployments), "no deployments created")

	deployments := deployResp.Deployments

	s.T().Logf("Cleaning up %d deployments", len(deployments))
	for _, createdDep := range deployments {
		// teardown lease
		_, err = deploycli.TxCloseDeploymentExec(
			s.validator.ClientCtx,
			keyTenant.GetAddress(),
			fmt.Sprintf("--owner=%s", createdDep.Groups[0].GroupID.Owner),
			fmt.Sprintf("--dseq=%v", createdDep.Deployment.DeploymentID.DSeq),
			fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
			fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
			fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
			fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		)
		s.Require().NoError(err)
		s.Require().NoError(s.waitForBlocksCommitted(1))
	}

	s.Require().NoError(s.waitForBlocksCommitted(3))
	// test query deployments with state filter closed
	resp, err = deploycli.QueryDeploymentsExec(
		s.validator.ClientCtx.WithOutputFormat("json"),
		"--state=closed",
	)
	s.Require().NoError(err)

	qResp := &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), qResp)
	s.Require().NoError(err)
	s.Require().True(len(qResp.Deployments) == len(deployResp.Deployments), "Deployment Close Failed")

	s.network.Cleanup()
}

func newestLease(leases mtypes.Leases) mtypes.Lease {
	result := mtypes.Lease{}
	assigned := false

	for _, lease := range leases {
		if !assigned {
			result = lease
			assigned = true
		} else if result.GetLeaseID().DSeq < lease.GetLeaseID().DSeq {
			result = lease
		}
	}

	return result
}

func getKubernetesIP() string {
	return os.Getenv("KUBE_NODE_IP")
}

func (s *IntegrationTestSuite) TestE2EContainerToContainer() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-c2c.yaml")
	s.Require().NoError(err)

	// Create Deployments
	_, err = deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(7))

	// Assert provider made bid and created lease; test query leases ---------
	resp, err := mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)

	s.Require().Len(leaseRes.Leases, len(s.prevLeases)+1)
	s.prevLeases = leaseRes.Leases

	lease := newestLease(leaseRes.Leases)
	lid := lease.LeaseID
	s.Require().Equal(s.keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider ----------------------------------------------
	_, err = ptestutil.TestSendManifest(s.validator.ClientCtx.WithOutputFormat("json"), lid.BidID(), deploymentPath)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))

	// Hit the endpoint to set a key in redis, foo = bar
	appURL := fmt.Sprintf("http://%s:%s/SET/foo/bar", s.appHost, s.appPort)

	const testHost = "webdistest.localhost"
	const attempts = 120
	httpResp := queryAppWithRetries(s.T(), appURL, testHost, attempts)
	bodyData, err := ioutil.ReadAll(httpResp.Body)
	s.Require().NoError(err)
	s.Require().Equal(`{"SET":[true,"OK"]}`, string(bodyData))

	// Hit the endpoint to read a key in redis, foo
	appURL = fmt.Sprintf("http://%s:%s/GET/foo", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts)
	bodyData, err = ioutil.ReadAll(httpResp.Body)
	s.Require().NoError(err)
	s.Require().Equal(`{"GET":"bar"}`, string(bodyData)) // Check that the value is bar
}

func (s *IntegrationTestSuite) TestE2EAppNodePort() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-nodeport.yaml")
	s.Require().NoError(err)

	// Create Deployments
	_, err = deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(7))

	// Assert provider made bid and created lease; test query leases ---------
	resp, err := mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)

	s.Require().Len(leaseRes.Leases, len(s.prevLeases)+1)
	s.prevLeases = leaseRes.Leases

	lease := newestLease(leaseRes.Leases)
	lid := lease.LeaseID
	s.Require().Equal(s.keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider ----------------------------------------------
	_, err = ptestutil.TestSendManifest(s.validator.ClientCtx.WithOutputFormat("json"), lid.BidID(), deploymentPath)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))

	// Get the lease status
	cmdResult, err := providerCmd.ProviderLeaseStatusExec(
		s.validator.ClientCtx,
		fmt.Sprintf("--%s=%v", "dseq", lid.DSeq),
		fmt.Sprintf("--%s=%v", "gseq", lid.GSeq),
		fmt.Sprintf("--%s=%v", "oseq", lid.OSeq),
		fmt.Sprintf("--%s=%v", "owner", lid.Owner),
		fmt.Sprintf("--%s=%v", "provider", lid.Provider))
	assert.NoError(s.T(), err)
	data := ctypes.LeaseStatus{}
	err = json.Unmarshal(cmdResult.Bytes(), &data)
	assert.NoError(s.T(), err)

	forwardedPort := uint16(0)
portLoop:
	for _, entry := range data.ForwardedPorts {
		for _, port := range entry {
			forwardedPort = port.ExternalPort
			break portLoop
		}
	}
	s.Require().NotEqual(uint16(0), forwardedPort)

	const maxAttempts = 60
	var recvData []byte
	var connErr error
	var conn net.Conn

	kubernetesIP := getKubernetesIP()
	if len(kubernetesIP) != 0 {
		for attempts := 0; attempts != maxAttempts; attempts++ {
			// Connect with a timeout so the test doesn't get stuck here
			conn, connErr = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", kubernetesIP, forwardedPort), 2*time.Second)
			// If an error, just wait and try again
			if connErr != nil {
				time.Sleep(time.Duration(500) * time.Millisecond)
				continue
			}
			break
		}

		// check that a connection was created without any error
		s.Require().NoError(connErr)
		// Read everything with a timeout
		err = conn.SetReadDeadline(time.Now().Add(time.Duration(10) * time.Second))
		s.Require().NoError(err)
		recvData, err = ioutil.ReadAll(conn)
		s.Require().NoError(err)
		s.Require().NoError(conn.Close())

		s.Require().Regexp("^.*hello world(?s:.)*$", string(recvData))
	}
}

func (s *IntegrationTestSuite) TestE2EDeploymentUpdate() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-updateA.yaml")
	s.Require().NoError(err)

	// Create Deployments
	_, err = deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(7))

	// Assert provider made bid and created lease; test query leases ---------
	resp, err := mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)

	s.Require().Len(leaseRes.Leases, len(s.prevLeases)+1)
	s.prevLeases = leaseRes.Leases

	lease := newestLease(leaseRes.Leases)
	lid := lease.LeaseID
	did := lease.GetLeaseID().DeploymentID()
	s.Require().Equal(s.keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider
	_, err = ptestutil.TestSendManifest(s.validator.ClientCtx.WithOutputFormat("json"), lid.BidID(), deploymentPath)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))

	appURL := fmt.Sprintf("http://%s:%s/", s.appHost, s.appPort)
	queryAppWithHostname(s.T(), appURL, 50, "testupdatea.localhost")

	deploymentPath, err = filepath.Abs("../x/deployment/testdata/deployment-v2-updateB.yaml")
	s.Require().NoError(err)

	_, err = deploycli.TxUpdateDeploymentExec(s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--owner=%s", lease.GetLeaseID().Owner),
		fmt.Sprintf("--dseq=%v", did.GetDSeq()),
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))

	// Send Updated Manifest to Provider
	_, err = ptestutil.TestSendManifest(s.validator.ClientCtx.WithOutputFormat("json"), lid.BidID(), deploymentPath)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))
	queryAppWithHostname(s.T(), appURL, 50, "testupdateb.localhost")

}

func (s *IntegrationTestSuite) TestE2EApp() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	// Create Deployments and assert query to assert
	tenantAddr := s.keyTenant.GetAddress().String()
	_, err = deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())

	// Test query deployments ---------------------------------------------
	resp, err := deploycli.QueryDeploymentsExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	deployResp := &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1, "Deployment Create Failed")
	deployments := deployResp.Deployments
	s.Require().Equal(tenantAddr, deployments[0].Deployment.DeploymentID.Owner)

	// test query deployment
	createdDep := deployments[0]
	resp, err = deploycli.QueryDeploymentExec(s.validator.ClientCtx.WithOutputFormat("json"), createdDep.Deployment.DeploymentID)
	s.Require().NoError(err)

	deploymentResp := dtypes.DeploymentResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), &deploymentResp)
	s.Require().NoError(err)
	s.Require().Equal(createdDep, deploymentResp)
	s.Require().NotEmpty(deploymentResp.Deployment.Version)

	// test query deployments with filters -----------------------------------
	resp, err = deploycli.QueryDeploymentsExec(
		s.validator.ClientCtx.WithOutputFormat("json"),
		fmt.Sprintf("--owner=%s", tenantAddr),
		fmt.Sprintf("--dseq=%v", createdDep.Deployment.DeploymentID.DSeq),
	)
	s.Require().NoError(err, "Error when fetching deployments with owner filter")

	deployResp = &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1)

	// Assert orders created by provider
	// test query orders
	resp, err = mcli.QueryOrdersExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	result := &mtypes.QueryOrdersResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	orders := result.Orders
	s.Require().Equal(tenantAddr, orders[0].OrderID.Owner)

	// Wait for then EndBlock to handle bidding and creating lease
	s.Require().NoError(s.waitForBlocksCommitted(6))

	// Assert provider made bid and created lease; test query leases ---------
	resp, err = mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.JSONMarshaler.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, len(s.prevLeases)+1)
	s.prevLeases = leaseRes.Leases
	lease := newestLease(leaseRes.Leases)
	lid := lease.LeaseID
	s.Require().Equal(s.keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider ----------------------------------------------
	_, err = ptestutil.TestSendManifest(s.validator.ClientCtx.WithOutputFormat("json"), lid.BidID(), deploymentPath)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(20))

	appURL := fmt.Sprintf("http://%s:%s/", s.appHost, s.appPort)
	queryApp(s.T(), appURL, 50)

	cmdResult, err := providerCmd.ProviderStatusExec(
		s.validator.ClientCtx,
		fmt.Sprintf("--%s=%v", "provider", lid.Provider))
	assert.NoError(s.T(), err)
	data := make(map[string]interface{})
	err = json.Unmarshal(cmdResult.Bytes(), &data)
	assert.NoError(s.T(), err)
	leaseCount, ok := data["cluster"].(map[string]interface{})["leases"]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), float64(1), leaseCount)

	// Read SDL into memory so each service can be checked
	deploymentSdl, err := sdl.ReadFile(deploymentPath)
	require.NoError(s.T(), err)
	mani, err := deploymentSdl.Manifest()
	require.NoError(s.T(), err)

	cmdResult, err = providerCmd.ProviderLeaseStatusExec(
		s.validator.ClientCtx,
		fmt.Sprintf("--%s=%v", "dseq", lid.DSeq),
		fmt.Sprintf("--%s=%v", "gseq", lid.GSeq),
		fmt.Sprintf("--%s=%v", "oseq", lid.OSeq),
		fmt.Sprintf("--%s=%v", "owner", lid.Owner),
		fmt.Sprintf("--%s=%v", "provider", lid.Provider))
	assert.NoError(s.T(), err)
	err = json.Unmarshal(cmdResult.Bytes(), &data)
	assert.NoError(s.T(), err)
	for _, group := range mani.GetGroups() {
		for _, service := range group.Services {
			serviceTotalCount, ok := data["services"].(map[string]interface{})[service.Name].(map[string]interface{})["total"]
			assert.True(s.T(), ok)
			assert.Greater(s.T(), serviceTotalCount, float64(0))
		}
	}

	for _, group := range mani.GetGroups() {
		for _, service := range group.Services {
			cmdResult, err = providerCmd.ProviderServiceStatusExec(
				s.validator.ClientCtx,
				fmt.Sprintf("--%s=%v", "dseq", lid.DSeq),
				fmt.Sprintf("--%s=%v", "gseq", lid.GSeq),
				fmt.Sprintf("--%s=%v", "oseq", lid.OSeq),
				fmt.Sprintf("--%s=%v", "owner", lid.Owner),
				fmt.Sprintf("--%s=%v", "provider", lid.Provider),
				fmt.Sprintf("--%s=%v", "service", service.Name))
			assert.NoError(s.T(), err)
			err = json.Unmarshal(cmdResult.Bytes(), &data)
			assert.NoError(s.T(), err)
			serviceTotalCount, ok := data["services"].(map[string]interface{})[service.Name].(map[string]interface{})["total"]
			assert.True(s.T(), ok)
			assert.Greater(s.T(), serviceTotalCount, float64(0))
		}
	}
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
