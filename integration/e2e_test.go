//go:build !mainnet
// +build !mainnet

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/golang-jwt/jwt/v4"

	akashclient "github.com/ovrclk/akash/pkg/client/clientset/versioned"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	providerCmd "github.com/ovrclk/akash/provider/cmd"

	"github.com/ovrclk/akash/provider/gateway/rest"
	"github.com/ovrclk/akash/sdl"
	clitestutil "github.com/ovrclk/akash/testutil/cli"
	mcli "github.com/ovrclk/akash/x/market/client/cli"

	"github.com/cosmos/cosmos-sdk/server"
	bankcli "github.com/cosmos/cosmos-sdk/x/bank/client/testutil"

	ccli "github.com/ovrclk/akash/x/cert/client/cli"
	"github.com/ovrclk/akash/x/provider/client/cli"
	types "github.com/ovrclk/akash/x/provider/types/v1beta2"

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
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

// IntegrationTestSuite wraps testing components
type IntegrationTestSuite struct {
	suite.Suite

	cfg         network.Config
	network     *network.Network
	validator   *network.Validator
	keyProvider keyring.Info
	keyTenant   keyring.Info

	group     *errgroup.Group
	ctx       context.Context
	ctxCancel context.CancelFunc

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
	cfg.MinGasPrices = fmt.Sprintf("0%s", testutil.CoinDenom)
	s.cfg = cfg
	s.network = network.New(s.T(), cfg)

	kb := s.network.Validators[0].ClientCtx.Keyring
	_, _, err := kb.NewMnemonic("keyBar", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	s.Require().NoError(err)
	_, _, err = kb.NewMnemonic("keyFoo", keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// address for provider to listen on
	_, port, err := server.FreeTCPAddr()
	require.NoError(s.T(), err)
	provHost := fmt.Sprintf("localhost:%s", port)
	provURL := url.URL{
		Host:   provHost,
		Scheme: "https",
	}
	// address for JWT server to listen on
	_, port, err = server.FreeTCPAddr()
	require.NoError(s.T(), err)
	jwtHost := fmt.Sprintf("localhost:%s", port)
	jwtURL := url.URL{
		Host:   jwtHost,
		Scheme: "https",
	}
	provFileStr := fmt.Sprintf(providerTemplate, provURL.String(), jwtURL.String())
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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), out)
	s.Require().NoError(err)
	s.Require().Len(out.Providers, 1, "Provider Creation Failed")
	providers := out.Providers
	s.Require().Equal(s.keyProvider.GetAddress().String(), providers[0].Owner)

	// test query provider
	createdProvider := providers[0]
	resp, err = cli.QueryProviderExec(localCtx, createdProvider.Owner)
	s.Require().NoError(err)

	var provider types.Provider
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), &provider)
	s.Require().NoError(err)
	s.Require().Equal(createdProvider, provider)

	// Run Provider service
	keyName := s.keyProvider.GetName()

	// Change the akash home directory for CLI to access the test keyring
	cliHome := strings.Replace(s.validator.ClientCtx.HomeDir, "simd", "simcli", 1)

	cctx := s.validator.ClientCtx

	// A context object to tie the lifetime of the provider & hostname operator to
	ctx, cancel := context.WithCancel(context.Background())
	s.ctxCancel = cancel

	s.group, s.ctx = errgroup.WithContext(ctx)

	// all command use viper which is meant for use by a single goroutine only
	// so wait for the provider to start before running the hostname operator

	s.group.Go(func() error {
		_, err := ptestutil.RunLocalProvider(ctx,
			cctx,
			cctx.ChainID,
			s.validator.RPCAddress,
			cliHome,
			keyName,
			provURL.Host,
			jwtURL.Host,
			fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
			"--deployment-runtime-class=none",
		)

		return err
	})

	// Run the hostname operator, after confirming the provider starts
	const maxAttempts = 30
	dialer := net.Dialer{
		Timeout: time.Second * 3,
	}

	attempts := 0
	s.T().Log("waiting for provider to run before starting hostname operator")
	for {
		conn, err := dialer.DialContext(s.ctx, "tcp", provHost)
		if err != nil {
			s.T().Logf("connecting to provider returned %v", err)
			_, ok := err.(net.Error)
			s.Require().True(ok, "error should be net error not %v", err)
			attempts++
			s.Require().Less(attempts, maxAttempts)
			time.Sleep(1 * time.Second)
			continue
		}
		_ = conn.Close() // Connected OK
		break
	}

	s.group.Go(func() error {
		s.T().Log("starting hostname operator for test")
		_, err := ptestutil.RunLocalHostnameOperator(s.ctx, cctx)
		s.Require().ErrorIs(err, context.Canceled)

		return nil
	})

	s.Require().NoError(s.network.WaitForNextBlock())
}

func (s *IntegrationTestSuite) TearDownTest() {
	s.T().Log("Cleaning up after E2E test")
	s.closeDeployments()
}

func (s *IntegrationTestSuite) closeDeployments() int {
	keyTenant, err := s.validator.ClientCtx.Keyring.Key("keyBar")
	s.Require().NoError(err)
	resp, err := deploycli.QueryDeploymentsExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)
	deployResp := &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), deployResp)
	s.Require().NoError(err)

	deployments := deployResp.Deployments

	s.T().Logf("Cleaning up %d deployments", len(deployments))
	for _, createdDep := range deployments {
		if createdDep.Deployment.State != dtypes.DeploymentActive {
			continue
		}
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
		clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())
	}

	return len(deployments)
}

func (s *IntegrationTestSuite) TearDownSuite() {
	s.T().Log("Cleaning up after E2E suite")
	n := s.closeDeployments()
	// test query deployments with state filter closed
	resp, err := deploycli.QueryDeploymentsExec(
		s.validator.ClientCtx.WithOutputFormat("json"),
		"--state=closed",
	)
	s.Require().NoError(err)

	qResp := &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), qResp)
	s.Require().NoError(err)
	s.Require().True(len(qResp.Deployments) == n, "Deployment Close Failed")

	s.network.Cleanup()

	// remove all entries of the provider host CRD
	cfgPath := path.Join(homedir.HomeDir(), ".kube", "config")

	restConfig, err := clientcmd.BuildConfigFromFlags("", cfgPath)
	s.Require().NoError(err)

	ac, err := akashclient.NewForConfig(restConfig)
	s.Require().NoError(err)
	const ns = "lease"
	propagation := metav1.DeletePropagationForeground
	err = ac.AkashV1().ProviderHosts(ns).DeleteCollection(s.ctx, metav1.DeleteOptions{
		TypeMeta:           metav1.TypeMeta{},
		GracePeriodSeconds: nil,
		Preconditions:      nil,
		OrphanDependents:   nil,
		PropagationPolicy:  &propagation,
		DryRun:             nil,
	}, metav1.ListOptions{

		LabelSelector:        `akash.network=true`,
		FieldSelector:        "",
		Watch:                false,
		AllowWatchBookmarks:  false,
		ResourceVersion:      "",
		ResourceVersionMatch: "",
		TimeoutSeconds:       nil,
		Limit:                0,
		Continue:             "",
	},
	)
	s.Require().NoError(err)

	time.Sleep(3 * time.Second) // Make sure hostname operator has time to delete ingress

	s.ctxCancel() // Stop context that provider & hostname operator are tied to

	_ = s.group.Wait()
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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Assert provider made bid and created lease; test query leases ---------
	resp, err := mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Assert provider made bid and created lease; test query leases ---------
	resp, err := mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Test query deployments ---------------------------------------------
	res, err = deploycli.QueryDeploymentsExec(cctxJSON)
	s.Require().NoError(err)

	deployResp := &dtypes.QueryDeploymentsResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1, "Deployment Create Failed")
	deployments := deployResp.Deployments
	s.Require().Equal(tenantAddr, deployments[0].Deployment.DeploymentID.Owner)

	// test query deployment
	createdDep := deployments[0]
	res, err = deploycli.QueryDeploymentExec(cctxJSON, createdDep.Deployment.DeploymentID)
	s.Require().NoError(err)

	deploymentResp := dtypes.QueryDeploymentResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), &deploymentResp)
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
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), deployResp)
	s.Require().NoError(err)
	s.Require().Len(deployResp.Deployments, 1)

	// Assert orders created by provider
	// test query orders
	res, err = mcli.QueryOrdersExec(cctxJSON)
	s.Require().NoError(err)

	result := &mtypes.QueryOrdersResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), result)
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
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), bidsRes)
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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	res, err = mcli.QueryLeasesExec(cctxJSON)
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), leaseRes)
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

func (s *E2EDeploymentUpdate) TestE2ELeaseShell() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2.yaml")
	s.Require().NoError(err)

	deploymentID := dtypes.DeploymentID{
		Owner: s.keyTenant.GetAddress().String(),
		DSeq:  uint64(104),
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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Assert provider made bid and created lease; test query leases ---------
	resp, err := mcli.QueryLeasesExec(s.validator.ClientCtx.WithOutputFormat("json"))
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(resp.Bytes(), leaseRes)
	s.Require().NoError(err)

	lease := newestLease(leaseRes.Leases)
	lID := lease.LeaseID

	s.Require().Equal(s.keyProvider.GetAddress().String(), lID.Provider)

	// Send Manifest to Provider
	_, err = ptestutil.TestSendManifest(
		s.validator.ClientCtx.WithOutputFormat("json"),
		lID.BidID(),
		deploymentPath,
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)

	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(2))

	extraArgs := []string{
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	}

	const attempts = 30
	const pollingPeriod = time.Second

	var out sdktest.BufferWriter

	i := 0
	for ; i != attempts; i++ {
		out, err = ptestutil.TestLeaseShell(s.validator.ClientCtx.WithOutputFormat("json"), extraArgs,
			lID, 0, false, false, "web", "/bin/echo", "foo")
		if err != nil {
			if errors.Is(err, rest.ErrLeaseShellProviderError) {
				s.T().Logf("encountered %v waiting before next attempt", err)
				time.Sleep(pollingPeriod)
				continue
			}

			// Fail now
			s.T().Fatalf("failed while trying to run lease-shell: %v", err)
		}
		require.NotNil(s.T(), out)
		break
	}
	require.NotEqual(s.T(), attempts, i, "failed to run lease shell after %d attempts", attempts)

	// Test failure cases now
	_, err = ptestutil.TestLeaseShell(s.validator.ClientCtx.WithOutputFormat("json"), extraArgs,
		lID, 0, false, false, "web", "/bin/baz", "foo")
	require.Error(s.T(), err)
	require.Regexp(s.T(), ".*command could not be executed because it does not exist.*", err.Error())

	_, err = ptestutil.TestLeaseShell(s.validator.ClientCtx.WithOutputFormat("json"), extraArgs,
		lID, 0, false, false, "web", "baz", "foo")
	require.Error(s.T(), err)
	require.Regexp(s.T(), ".*command could not be executed because it does not exist.*", err.Error())

	_, err = ptestutil.TestLeaseShell(s.validator.ClientCtx.WithOutputFormat("json"), extraArgs,
		lID, 0, false, false, "web", "baz", "foo")
	require.Error(s.T(), err)
	require.Regexp(s.T(), ".*command could not be executed because it does not exist.*", err.Error())

	_, err = ptestutil.TestLeaseShell(s.validator.ClientCtx.WithOutputFormat("json"), extraArgs,
		lID, 99, false, false, "web", "/bin/echo", "foo")
	require.Error(s.T(), err)
	require.Regexp(s.T(), ".*pod index out of range.*", err.Error())

	_, err = ptestutil.TestLeaseShell(s.validator.ClientCtx.WithOutputFormat("json"), extraArgs,
		lID, 99, false, false, "web", "/bin/echo", "foo")
	require.Error(s.T(), err)
	require.Regexp(s.T(), ".*pod index out of range.*", err.Error())

	_, err = ptestutil.TestLeaseShell(s.validator.ClientCtx.WithOutputFormat("json"), extraArgs,
		lID, 0, false, false, "web", "/bin/cat", "/foo")
	require.Error(s.T(), err)
	require.Regexp(s.T(), ".*remote process exited with code 1.*", err.Error())

	_, err = ptestutil.TestLeaseShell(s.validator.ClientCtx.WithOutputFormat("json"), extraArgs,
		lID, 99, false, false, "notaservice", "/bin/echo", "/foo")
	require.Error(s.T(), err)
	require.Regexp(s.T(), ".*no such service exists with that name.*", err.Error())

}

func (s *E2EApp) TestE2EMigrateHostname() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-migrate.yaml")
	s.Require().NoError(err)

	cctxJSON := s.validator.ClientCtx.WithOutputFormat("json")

	deploymentID := dtypes.DeploymentID{
		Owner: s.keyTenant.GetAddress().String(),
		DSeq:  uint64(105),
	}

	// Create Deployments and assert query to assert
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
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Wait for then EndBlock to handle bidding and creating lease
	s.Require().NoError(s.waitForBlocksCommitted(15))

	// Assert provider made bid and created lease; test query leases
	res, err = mcli.QueryBidsExec(cctxJSON)
	s.Require().NoError(err)
	bidsRes := &mtypes.QueryBidsResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), bidsRes)
	s.Require().NoError(err)
	selectedIdx := -1
	for i, bidEntry := range bidsRes.Bids {
		bid := bidEntry.GetBid()
		if bid.GetBidID().DeploymentID().Equals(deploymentID) {
			selectedIdx = i
			break
		}
	}
	s.Require().NotEqual(selectedIdx, -1)
	bid := bidsRes.Bids[selectedIdx].GetBid()

	res, err = mcli.TxCreateLeaseExec(
		cctxJSON,
		bid.BidID,
		s.keyTenant.GetAddress(),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(6))
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	res, err = mcli.QueryLeasesExec(cctxJSON)
	s.Require().NoError(err)

	leaseRes := &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), leaseRes)
	s.Require().NoError(err)
	selectedIdx = -1
	for idx, leaseEntry := range leaseRes.Leases {
		lease := leaseEntry.GetLease()
		if lease.GetLeaseID().DeploymentID().Equals(deploymentID) {
			selectedIdx = idx
			break
		}
	}
	s.Require().NotEqual(selectedIdx, -1)

	lease := leaseRes.Leases[selectedIdx].GetLease()
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

	const primaryHostname = "leaveme.com"
	const secondaryHostname = "migrateme.com"

	appURL := fmt.Sprintf("http://%s:%s/", s.appHost, s.appPort)
	queryAppWithHostname(s.T(), appURL, 50, primaryHostname)
	queryAppWithHostname(s.T(), appURL, 50, secondaryHostname)

	cmdResult, err := providerCmd.ProviderStatusExec(s.validator.ClientCtx, lid.Provider)
	assert.NoError(s.T(), err)
	data := make(map[string]interface{})
	err = json.Unmarshal(cmdResult.Bytes(), &data)
	assert.NoError(s.T(), err)
	leaseCount, ok := data["cluster"].(map[string]interface{})["leases"]
	assert.True(s.T(), ok)
	assert.Equal(s.T(), float64(1), leaseCount)

	// Create another deployment, use the same exact SDL

	secondDeploymentID := dtypes.DeploymentID{
		Owner: s.keyTenant.GetAddress().String(),
		DSeq:  uint64(106),
	}

	res, err = deploycli.TxCreateDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		fmt.Sprintf("--%s", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(20))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
		fmt.Sprintf("--dseq=%v", secondDeploymentID.DSeq),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.network.WaitForNextBlock())
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	// Wait for then EndBlock to handle bidding and creating lease
	s.Require().NoError(s.waitForBlocksCommitted(15))

	// Assert provider made bid and created lease; test query leases
	res, err = mcli.QueryBidsExec(cctxJSON)
	s.Require().NoError(err)
	bidsRes = &mtypes.QueryBidsResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), bidsRes)
	s.Require().NoError(err)

	selectedIdx = -1
	for i, bidEntry := range bidsRes.Bids {
		bid := bidEntry.GetBid()
		if bid.GetBidID().DeploymentID().Equals(secondDeploymentID) {
			selectedIdx = i
			break
		}
	}
	s.Require().NotEqual(selectedIdx, -1)
	bid = bidsRes.Bids[selectedIdx].GetBid()

	res, err = mcli.TxCreateLeaseExec(
		cctxJSON,
		bid.BidID,
		s.keyTenant.GetAddress(),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(6))
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	res, err = mcli.QueryLeasesExec(cctxJSON)
	s.Require().NoError(err)

	leaseRes = &mtypes.QueryLeasesResponse{}
	err = s.validator.ClientCtx.Codec.UnmarshalJSON(res.Bytes(), leaseRes)
	s.Require().NoError(err)
	selectedIdx = -1
	for idx, leaseEntry := range leaseRes.Leases {
		lease := leaseEntry.GetLease()
		if lease.GetLeaseID().DeploymentID().Equals(secondDeploymentID) {
			selectedIdx = idx
			break
		}
	}
	s.Require().NotEqual(selectedIdx, -1)

	secondLease := leaseRes.Leases[selectedIdx].GetLease()
	secondLID := secondLease.LeaseID
	s.Require().Equal(s.keyProvider.GetAddress().String(), lid.Provider)

	// Send Manifest to Provider ----------------------------------------------
	_, err = ptestutil.TestSendManifest(
		cctxJSON,
		secondLID.BidID(),
		deploymentPath,
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(20))

	// migrate hostname
	_, err = ptestutil.TestMigrateHostname(cctxJSON, lid, secondDeploymentID.DSeq, secondaryHostname,
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir))
	s.Require().NoError(err)

	time.Sleep(10 * time.Second) // update happens in kube async

	// Get the lease status and confirm hostname is present
	cmdResult, err = providerCmd.ProviderLeaseStatusExec(
		s.validator.ClientCtx,
		fmt.Sprintf("--%s=%v", "dseq", secondLID.DSeq),
		fmt.Sprintf("--%s=%v", "gseq", secondLID.GSeq),
		fmt.Sprintf("--%s=%v", "oseq", secondLID.OSeq),
		fmt.Sprintf("--%s=%v", "provider", secondLID.Provider),
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)
	assert.NoError(s.T(), err)
	leaseStatusData := ctypes.LeaseStatus{}
	err = json.Unmarshal(cmdResult.Bytes(), &leaseStatusData)
	assert.NoError(s.T(), err)

	hostnameFound := false
	for _, service := range leaseStatusData.Services {
		for _, serviceURI := range service.URIs {
			if serviceURI == secondaryHostname {
				hostnameFound = true
				break
			}
		}
	}
	s.Require().True(hostnameFound, "could not find hostname")

	// close first deployment & lease
	res, err = deploycli.TxCloseDeploymentExec(
		s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		fmt.Sprintf("--owner=%s", deploymentID.GetOwner()),
		fmt.Sprintf("--dseq=%v", deploymentID.GetDSeq()),
		fmt.Sprintf("--%s=true", flags.FlagSkipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, flags.BroadcastBlock),
		fmt.Sprintf("--%s=%s", flags.FlagFees, sdk.NewCoins(sdk.NewCoin(s.cfg.BondDenom, sdk.NewInt(10))).String()),
		fmt.Sprintf("--gas=%d", flags.DefaultGasLimit),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(1))
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	time.Sleep(10 * time.Second) // Make sure provider has time to close the lease
	cmdResult, err = providerCmd.ProviderLeaseStatusExec(
		s.validator.ClientCtx,
		fmt.Sprintf("--%s=%v", "dseq", lid.DSeq),
		fmt.Sprintf("--%s=%v", "gseq", lid.GSeq),
		fmt.Sprintf("--%s=%v", "oseq", lid.OSeq),
		fmt.Sprintf("--%s=%v", "provider", lid.Provider),
		fmt.Sprintf("--%s=%s", flags.FlagFrom, s.keyTenant.GetAddress().String()),
		fmt.Sprintf("--%s=%s", flags.FlagHome, s.validator.ClientCtx.HomeDir),
	)
	s.Require().NoError(err)
	s.Require().Len(cmdResult.Bytes(), 0)

	// confirm hostname still reachable, on the hostname it was migrated to
	queryAppWithHostname(s.T(), appURL, 50, secondaryHostname)
}

func (s *E2EApp) TestE2EJwtServerAuthenticate() {
	cctx := s.validator.ClientCtx
	provider := s.keyProvider.GetAddress().String()
	tenant := s.keyTenant.GetAddress().String()

	buf, err := ptestutil.TestJwtServerAuthenticate(cctx, provider, tenant)
	s.Require().NoError(err)

	var claims rest.ClientCustomClaims
	_, _, err = (&jwt.Parser{}).ParseUnverified(buf.String(), &claims)
	require.NoError(s.T(), err)

	require.Equal(s.T(), provider, claims.Issuer)
	require.Equal(s.T(), tenant, claims.Subject)
	require.NotEmpty(s.T(), claims.AkashNamespace.V1.CertSerialNumber)
}

func TestIntegrationTestSuite(t *testing.T) {
	integrationTestOnly(t)
	suite.Run(t, new(E2EContainerToContainer))
	suite.Run(t, new(E2EAppNodePort))
	suite.Run(t, new(E2EDeploymentUpdate))
	suite.Run(t, new(E2EApp))
	suite.Run(t, new(E2EPersistentStorageDefault))
	suite.Run(t, new(E2EPersistentStorageBeta2))
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

	appURL := fmt.Sprintf("http://%s:%s/", host, appPort)
	queryApp(t, appURL, 1)
}
