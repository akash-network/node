// +build e2e,integration,!mainnet

package integration

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/tests"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2EApp(t *testing.T) {
	host, appPort := appEnv(t)

	t.Parallel()
	f := InitFixtures(t)
	defer f.Cleanup() // NOTE: defer statement ordering matters.

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
	tests.WaitForNextNBlocksTM(1, f.Port)

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
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments, err := f.QueryDeployments()
	require.NoError(t, err)
	require.Len(t, deployments, 1, "Deployment Create Failed")
	require.Equal(t, barAddr.String(), deployments[0].Deployment.DeploymentID.Owner.String())

	// test query deployment
	createdDep := deployments[0]
	deployment := f.QueryDeployment(createdDep.Deployment.DeploymentID)
	require.Equal(t, createdDep, deployment)

	tests.WaitForNextNBlocksTM(1, f.Port)
	orders, err := f.QueryOrders()
	require.NoError(t, err)
	require.Len(t, orders, 1, "Order creation failed")
	require.Equal(t, barAddr.String(), orders[0].OrderID.Owner.String())

	// Assert that there are no leases yet
	leases, err := f.QueryLeases()
	require.NoError(t, err)
	require.Len(t, leases, 0, "no Leases should be created yet")

	// Wait for then EndBlock to handle bidding and creating lease
	tests.WaitForNextNBlocksTM(5, f.Port)

	// Assert provider made bid
	leases, err = f.QueryLeases()
	require.NoError(t, err)
	require.Len(t, leases, 1, "Lease should be created after bidding completes")
	lease := leases[0]

	// Provide manifest to provider service
	f.SendManifest(lease, deploymentOvrclkApp)

	// Wait for App to deploy
	tests.WaitForNextNBlocksTM(5, f.Port)

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

// Assert provider launches app in kind cluster
func queryApp(t *testing.T, appURL string) {
	req, err := http.NewRequest("GET", appURL, nil)
	require.NoError(t, err)
	req.Host = "hello.localhost" // NOTE: cannot be inserted as a req.Header element, that is overwritten by this req.Host field.
	req.Header.Add("Cache-Control", "no-cache")
	req.Header.Add("Connection", "keep-alive")
	req.Header.Add("Accept", "text/html,application/xhtml+xml,*/*")
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
