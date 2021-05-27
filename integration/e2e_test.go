// +build !mainnet

package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/server"
	bankcli "github.com/cosmos/cosmos-sdk/x/bank/client/testutil"
	"github.com/stretchr/testify/assert"

	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	providerCmd "github.com/ovrclk/akash/provider/cmd"
	"github.com/ovrclk/akash/sdl"
	ccli "github.com/ovrclk/akash/x/cert/client/cli"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	"github.com/ovrclk/akash/x/provider/client/cli"
	"github.com/ovrclk/akash/x/provider/types"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"

	ptestutil "github.com/ovrclk/akash/provider/testutil"
	"github.com/ovrclk/akash/testutil"
	deploycli "github.com/ovrclk/akash/x/deployment/client/cli"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
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

	appHost string
	appPort string
}

type E2EContainerToContainer struct {
	IntegrationTestSuite
}

type E2EAppNodePort struct {
	IntegrationTestSuite
}

type E2EDeploymentUpdate struct {
	IntegrationTestSuite
}

type E2EApp struct {
	IntegrationTestSuite
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
	_, _, err := kb.NewMnemonic("keyBar", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	s.Require().NoError(err)
	_, _, err = kb.NewMnemonic("keyFoo", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	s.Require().NoError(err)

	// Wait for the network to start
	_, err = s.network.WaitForHeight(1)
	s.Require().NoError(err)

	//
	s.validator = s.network.Validators[0]

	// Send coins value
	sendTokens := sdk.NewCoin(s.cfg.BondDenom, mtypes.DefaultBidMinDeposit.Amount.MulRaw(4))

	// Setup a Provider key
	s.keyProvider, err = s.validator.ClientCtx.Keyring.Key("keyFoo")
	s.Require().NoError(err)

	// give provider some coins
	res, err := bankcli.MsgSendExec(
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
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Set up second tenant key
	s.keyTenant, err = s.validator.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)

	// give tenant some coins too
	res, err = bankcli.MsgSendExec(
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
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// address for provider to listen on
	_, port, err := server.FreeTCPAddr()
	require.NoError(s.T(), err)
	provHost := fmt.Sprintf("localhost:%s", port)
	provURL := url.URL{
		Host:   provHost,
		Scheme: "https",
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
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Create provider's certificate
	_, err = ccli.TxCreateServerExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		"localhost",
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Create tenant's certificate
	_, err = ccli.TxCreateServerExec(
		s.validator.ClientCtx,
		s.keyProvider.GetAddress(),
		"localhost",
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	pemSrc := fmt.Sprintf("%s/%s.pem", s.validator.ClientCtx.HomeDir, s.keyProvider.GetAddress().String())
	pemDst := fmt.Sprintf("%s/%s.pem", strings.Replace(s.validator.ClientCtx.HomeDir, "simd", "simcli", 1), s.keyProvider.GetAddress().String())
	input, err := ioutil.ReadFile(pemSrc)
	s.Require().NoError(err)

	err = ioutil.WriteFile(pemDst, input, 0400)
	s.Require().NoError(err)

	pemSrc = fmt.Sprintf("%s/%s.pem", s.validator.ClientCtx.HomeDir, s.keyTenant.GetAddress().String())
	pemDst = fmt.Sprintf("%s/%s.pem", strings.Replace(s.validator.ClientCtx.HomeDir, "simd", "simcli", 1), s.keyTenant.GetAddress().String())
	input, err = ioutil.ReadFile(pemSrc)
	s.Require().NoError(err)

	err = ioutil.WriteFile(pemDst, input, 0400)
	s.Require().NoError(err)

	localCtx := s.validator.ClientCtx.WithOutputFormat("json")
	// test query providers
	resp, err := cli.QueryProvidersExec(localCtx)
	s.Require().NoError(err)

	out := &types.QueryProvidersResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(s.keyProvider.GetAddress().String(), providers[0].Owner)

	// test query provider
	createdProvider := providers[0]
	resp, err = cli.QueryProviderExec(localCtx, createdProvider.Owner)
	s.Require().NoError(err)

	var provider types.Provider
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(resp.Bytes(), &provider)
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
			"--deployment-runtime-class=none",
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
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(resp.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().False(0 == len(deployResp.Deployments), "no deployments created")

	deployments := deployResp.Deployments

	s.T().Logf("Cleaning up %d deployments", len(deployments))
	for _, createdDep := range deployments {
		// teardown lease
		res, err := deploycli.TxCloseDeploymentExec(
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
		validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())
	}

	// test query deployments with state filter closed
	resp, err = deploycli.QueryDeploymentsExec(
		s.validator.ClientCtx.WithOutputFormat("json"),
		"--state=closed",
	)
	s.Require().NoError(err)

	qResp := &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(resp.Bytes(), qResp)
	s.Require().NoError(err)
	s.Require().True(len(qResp.Deployments) == len(deployResp.Deployments), "Deployment Close Failed")

	s.network.Cleanup()
}

func newestLease(leases []mtypes.QueryLeaseResponse) mtypes.Lease {
	result := mtypes.Lease{}
	assigned := false

	for _, lease := range leases {
		if !assigned {
			result = lease.Lease
			assigned = true
		} else if result.GetLeaseID().DSeq < lease.Lease.GetLeaseID().DSeq {
			result = lease.Lease
		}
	}

	return result
}

func getKubernetesIP() string {
	return os.Getenv("KUBE_NODE_IP")
}

func (s *E2EContainerToContainer) TestE2EContainerToContainer() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-c2c.yaml")
	s.Require().NoError(err)

	deploymentID := dtypes.DeploymentID{
		Owner: s.keyTenant.GetAddress().String(),
		DSeq:  uint64(100),
	}

	// Create Deployments
	res, err := deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--deposit=%s", dtypes.DefaultDeploymentMinDeposit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(7))
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	bidID := mtypes.MakeBidID(
		mtypes.MakeOrderID(dtypes.MakeGroupID(deploymentID, 1), 1),
		s.keyProvider.GetAddress(),
	)

	// check bid
	_, err = mcli.QueryBidExec(s.validator.ClientCtx, bidID)
	s.Require().NoError(err)

	// create lease
	_, err = mcli.TxCreateLeaseExec(
		s.validator.ClientCtx,
		bidID,
		s.keyTenant.GetAddress(),
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	lid := bidID.LeaseID()

	// Send Manifest to Provider ----------------------------------------------
	_, err = ptestutil.TestSendManifest(
		s.validator.ClientCtx.WithOutputFormat("json"),
		lid.BidID(),
		deploymentPath,
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)
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

func (s *E2EAppNodePort) TestE2EAppNodePort() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-nodeport.yaml")
	s.Require().NoError(err)

	deploymentID := dtypes.DeploymentID{
		Owner: s.keyTenant.GetAddress().String(),
		DSeq:  uint64(101),
	}

	// Create Deployments
	res, err := deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(3))
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	bidID := mtypes.MakeBidID(
		mtypes.MakeOrderID(dtypes.MakeGroupID(deploymentID, 1), 1),
		s.keyProvider.GetAddress(),
	)
	// check bid
	_, err = mcli.QueryBidExec(s.validator.ClientCtx, bidID)
	s.Require().NoError(err)

	// create lease
	_, err = mcli.TxCreateLeaseExec(
		s.validator.ClientCtx,
		bidID,
		s.keyTenant.GetAddress(),
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Assert provider made bid and created lease; test query leases ---------
	resp, err := mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)

	lease := newestLease(leaseRes.Leases)
	lid := lease.LeaseID
	s.Require().Equal(s.keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider ----------------------------------------------
	_, err = ptestutil.TestSendManifest(
		s.validator.ClientCtx.WithOutputFormat("json"),
		lid.BidID(),
		deploymentPath,
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)

	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))

	// Get the lease status
	cmdResult, err := providerCmd.ProviderLeaseStatusExec(
		s.validator.ClientCtx,
		fmt.Sprintf("--%s=%v", "dseq", lid.DSeq),
		fmt.Sprintf("--%s=%v", "gseq", lid.GSeq),
		fmt.Sprintf("--%s=%v", "oseq", lid.OSeq),
		fmt.Sprintf("--%s=%v", "provider", lid.Provider),
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)
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

func (s *E2EDeploymentUpdate) TestE2EDeploymentUpdate() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-updateA.yaml")
	s.Require().NoError(err)

	deploymentID := dtypes.DeploymentID{
		Owner: s.keyTenant.GetAddress().String(),
		DSeq:  uint64(102),
	}

	// Create Deployments
	res, err := deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(3))
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	bidID := mtypes.MakeBidID(
		mtypes.MakeOrderID(dtypes.MakeGroupID(deploymentID, 1), 1),
		s.keyProvider.GetAddress(),
	)
	// check bid
	_, err = mcli.QueryBidExec(s.validator.ClientCtx, bidID)
	s.Require().NoError(err)

	// create lease
	_, err = mcli.TxCreateLeaseExec(
		s.validator.ClientCtx,
		bidID,
		s.keyTenant.GetAddress(),
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Assert provider made bid and created lease; test query leases ---------
	resp, err := mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)

	s.Require().Len(leaseRes.Leases, 1)

	lease := newestLease(leaseRes.Leases)
	lid := lease.LeaseID
	did := lease.GetLeaseID().DeploymentID()
	s.Require().Equal(s.keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider
	_, err = ptestutil.TestSendManifest(
		s.validator.ClientCtx.WithOutputFormat("json"),
		lid.BidID(),
		deploymentPath,
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)

	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))

	appURL := fmt.Sprintf("http://%s:%s/", s.appHost, s.appPort)
	queryAppWithHostname(s.T(), appURL, 50, "testupdatea.localhost")

	deploymentPath, err = filepath.Abs("../x/deployment/testdata/deployment-v2-updateB.yaml")
	s.Require().NoError(err)

	res, err = deploycli.TxUpdateDeploymentExec(s.validator.ClientCtx,
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
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Send Updated Manifest to Provider
	_, err = ptestutil.TestSendManifest(
		s.validator.ClientCtx.WithOutputFormat("json"),
		lid.BidID(),
		deploymentPath,
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))
	queryAppWithHostname(s.T(), appURL, 50, "testupdateb.localhost")
}

func (s *E2EApp) TestE2EApp() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	cctxJSON := s.validator.ClientCtx.WithOutputFormat("json")

	deploymentID := dtypes.DeploymentID{
		Owner: s.keyTenant.GetAddress().String(),
		DSeq:  uint64(103),
	}

	// Create Deployments and assert query to assert
	tenantAddr := s.keyTenant.GetAddress().String()
	res, err := deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", deploymentID.DSeq),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Test query deployments ---------------------------------------------
	res, err = deploycli.QueryDeploymentsExec(cctxJSON)
	s.Require().NoError(err)

	deployResp := &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(res.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1, "Deployment Create Failed")
	deployments := deployResp.Deployments
	s.Require().Equal(tenantAddr, deployments[0].Deployment.DeploymentID.Owner)

	// test query deployment
	createdDep := deployments[0]
	res, err = deploycli.QueryDeploymentExec(cctxJSON, createdDep.Deployment.DeploymentID)
	s.Require().NoError(err)

	deploymentResp := dtypes.QueryDeploymentResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(res.Bytes(), &deploymentResp)
	s.Require().NoError(err)
	s.Require().Equal(createdDep, deploymentResp)
	s.Require().NotEmpty(deploymentResp.Deployment.Version)

	// test query deployments with filters -----------------------------------
	res, err = deploycli.QueryDeploymentsExec(
		s.validator.ClientCtx.WithOutputFormat("json"),
		fmt.Sprintf("--owner=%s", tenantAddr),
		fmt.Sprintf("--dseq=%v", createdDep.Deployment.DeploymentID.DSeq),
	)
	s.Require().NoError(err, "Error when fetching deployments with owner filter")

	deployResp = &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(res.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1)

	// Assert orders created by provider
	// test query orders
	res, err = mcli.QueryOrdersExec(cctxJSON)
	s.Require().NoError(err)

	result := &mtypes.QueryOrdersResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(res.Bytes(), result)
	s.Require().NoError(err)
	s.Require().Len(result.Orders, 1)
	orders := result.Orders
	s.Require().Equal(tenantAddr, orders[0].OrderID.Owner)

	// Wait for then EndBlock to handle bidding and creating lease
	s.Require().NoError(s.waitForBlocksCommitted(15))

	// Assert provider made bid and created lease; test query leases
	// Assert provider made bid and created lease; test query leases
	res, err = mcli.QueryBidsExec(cctxJSON)
	s.Require().NoError(err)
	bidsRes := &mtypes.QueryBidsResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(res.Bytes(), bidsRes)
	s.Require().NoError(err)
	s.Require().Len(bidsRes.Bids, 1)

	res, err = mcli.TxCreateLeaseExec(
		cctxJSON,
		bidsRes.Bids[0].Bid.BidID,
		s.keyTenant.GetAddress(),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(6))
	validateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	res, err = mcli.QueryLeasesExec(cctxJSON)
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.JSONCodec.UnmarshalJSON(res.Bytes(), leaseRes)
	s.Require().NoError(err)
	s.Require().Len(leaseRes.Leases, 1)

	lease := newestLease(leaseRes.Leases)
	lid := lease.LeaseID
	s.Require().Equal(s.keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider ----------------------------------------------
	_, err = ptestutil.TestSendManifest(
		cctxJSON,
		lid.BidID(),
		deploymentPath,
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(20))

	appURL := fmt.Sprintf("http://%s:%s/", s.appHost, s.appPort)
	queryApp(s.T(), appURL, 50)

	cmdResult, err := providerCmd.ProviderStatusExec(s.validator.ClientCtx, lid.Provider)
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
		fmt.Sprintf("--%s=%v", "provider", lid.Provider),
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)
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
				fmt.Sprintf("--%s=%v", "provider", lid.Provider),
				fmt.Sprintf("--%s=%v", "service", service.Name),
				fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
				fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
			)
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
	integrationTestOnly(t)
	suite.Run(t, new(E2EContainerToContainer))
	suite.Run(t, new(E2EAppNodePort))
	suite.Run(t, new(E2EDeploymentUpdate))
	suite.Run(t, new(E2EApp))
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
	integrationTestOnly(t)
	host, appPort := appEnv(t)

	appURL := fmt.Sprintf("https://%s:%s/", host, appPort)
	queryApp(t, appURL, 1)
}
