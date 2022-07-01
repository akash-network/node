package integration

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"time"

	"github.com/cosmos/cosmos-sdk/client/flags"
	uuid "github.com/satori/go.uuid"

	ptestutil "github.com/ovrclk/akash/provider/testutil"
	clitestutil "github.com/ovrclk/akash/testutil/cli"
	deploycli "github.com/ovrclk/akash/x/deployment/client/cli"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type E2EPersistentStorageDefault struct {
	IntegrationTestSuite
}

type E2EPersistentStorageBeta2 struct {
	IntegrationTestSuite
}

type E2EPersistentStorageDeploymentUpdate struct {
	IntegrationTestSuite
}

func (s *E2EPersistentStorageDefault) TestDefaultStorageClass() {
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-storage-default.yaml")
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
		cliGlobalFlags(fmt.Sprintf("--deposit=%s", dtypes.DefaultDeploymentMinDeposit),
			fmt.Sprintf("--dseq=%v", deploymentID.DSeq))...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(7))
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	bidID := mtypes.MakeBidID(
		mtypes.MakeOrderID(dtypes.MakeGroupID(deploymentID, 1), 1),
		s.keyProvider.GetAddress(),
	)

	_, err = mcli.QueryBidExec(s.validator.ClientCtx, bidID)
	s.Require().NoError(err)

	_, err = mcli.TxCreateLeaseExec(
		s.validator.ClientCtx,
		bidID,
		s.keyTenant.GetAddress(),
		cliGlobalFlags()...,
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
	appURL := fmt.Sprintf("http://webdistest.localhost:%s/GET/value", s.appPort)

	const testHost = "webdistest.localhost"
	const attempts = 120
	httpResp := queryAppWithRetries(s.T(), appURL, testHost, attempts)
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)

	bodyData, err := ioutil.ReadAll(httpResp.Body)
	s.Require().NoError(err)
	s.Require().Equal(`default`, string(bodyData))

	testData := uuid.NewV4()

	// Hit the endpoint to read a key in redis, foo
	appURL = fmt.Sprintf("http://%s:%s/SET/value", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts, queryWithBody([]byte(testData.String())))
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)

	appURL = fmt.Sprintf("http://%s:%s/GET/value", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts)
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)

	bodyData, err = ioutil.ReadAll(httpResp.Body)
	s.Require().NoError(err)
	s.Require().Equal(testData.String(), string(bodyData))

	// send signal for pod to die
	appURL = fmt.Sprintf("http://%s:%s/kill", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts)
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)

	// give kube to to reschedule pod
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		cancel()
		return
	}
	cancel()

	appURL = fmt.Sprintf("http://%s:%s/GET/value", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts)
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)
	bodyData, err = ioutil.ReadAll(httpResp.Body)
	s.Require().NoError(err)
	s.Require().Equal(testData.String(), string(bodyData))
}

func (s *E2EPersistentStorageBeta2) TestDedicatedStorageClass() {
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-storage-beta2.yaml")
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
		cliGlobalFlags(fmt.Sprintf("--deposit=%s", dtypes.DefaultDeploymentMinDeposit),
			fmt.Sprintf("--dseq=%v", deploymentID.DSeq))...,
	)
	s.Require().NoError(err)
	s.Require().NoError(s.waitForBlocksCommitted(7))
	clitestutil.ValidateTxSuccessful(s.T(), s.validator.ClientCtx, res.Bytes())

	bidID := mtypes.MakeBidID(
		mtypes.MakeOrderID(dtypes.MakeGroupID(deploymentID, 1), 1),
		s.keyProvider.GetAddress(),
	)

	_, err = mcli.QueryBidExec(s.validator.ClientCtx, bidID)
	s.Require().NoError(err)

	_, err = mcli.TxCreateLeaseExec(
		s.validator.ClientCtx,
		bidID,
		s.keyTenant.GetAddress(),
		cliGlobalFlags()...,
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
	appURL := fmt.Sprintf("http://%s:%s/GET/value", s.appHost, s.appPort)

	const testHost = "webdistest.localhost"
	const attempts = 120
	httpResp := queryAppWithRetries(s.T(), appURL, testHost, attempts)
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)

	bodyData, err := ioutil.ReadAll(httpResp.Body)
	s.Require().NoError(err)
	s.Require().Equal(`default`, string(bodyData))
	testData := uuid.NewV4()

	// Hit the endpoint to read a key in redis, foo
	appURL = fmt.Sprintf("http://%s:%s/SET/value", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts, queryWithBody([]byte(testData.String())))
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)

	appURL = fmt.Sprintf("http://%s:%s/GET/value", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts)
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)
	bodyData, err = ioutil.ReadAll(httpResp.Body)
	s.Require().NoError(err)
	s.Require().Equal(testData.String(), string(bodyData))

	// send signal for pod to die
	appURL = fmt.Sprintf("http://%s:%s/kill", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts)
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)

	// give kube to to reschedule pod
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		cancel()
		return
	}
	cancel()

	appURL = fmt.Sprintf("http://%s:%s/GET/value", s.appHost, s.appPort)
	httpResp = queryAppWithRetries(s.T(), appURL, testHost, attempts)
	s.Require().Equal(http.StatusOK, httpResp.StatusCode)

	bodyData, err = ioutil.ReadAll(httpResp.Body)
	s.Require().NoError(err)
	s.Require().Equal(testData.String(), string(bodyData))
}

func (s *E2EPersistentStorageDeploymentUpdate) TestDeploymentUpdate() {
	// create a deployment
	deploymentPath, err := filepath.Abs("../x/deployment/testdata/deployment-v2-storage-updateA.yaml")
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
		cliGlobalFlags(fmt.Sprintf("--dseq=%v", deploymentID.DSeq))...,
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
		cliGlobalFlags()...,
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

	deploymentPath, err = filepath.Abs("../x/deployment/testdata/deployment-v2-storage-updateB.yaml")
	s.Require().NoError(err)

	res, err = deploycli.TxUpdateDeploymentExec(s.validator.ClientCtx,
		s.keyTenant.GetAddress(),
		deploymentPath,
		cliGlobalFlags(fmt.Sprintf("--owner=%s", lease.GetLeaseID().Owner),
			fmt.Sprintf("--dseq=%v", did.GetDSeq()))...,
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
	queryAppWithHostname(s.T(), appURL, 50, "testupdatea.localhost")
	queryAppWithHostname(s.T(), appURL, 50, "testupdateb.localhost")
}
