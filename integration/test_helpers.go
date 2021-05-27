package integration

import (
	"bytes"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	providerTemplate = `host: %s
attributes:
  - key: region
    value: us-west
  - key: moniker
    value: akash
`
)

// skip integration-only tests.
// using build tags breaks tooling for compilation, etc...
func integrationTestOnly(t testing.TB) {
	t.Helper()
	val, found := os.LookupEnv("TEST_INTEGRATION")
	if !found || val != "true" {
		t.Skip("SKIPPING INTEGRATION TEST")
	}
}

func queryAppWithRetries(t *testing.T, appURL string, appHost string, limit int) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", appURL, nil)
	require.NoError(t, err)
	req.Host = appHost
	req.Header.Add("Cache-Control", "no-cache")
	req.Header.Add("Connection", "keep-alive")
	tr := &http.Transport{
		DisableKeepAlives: false,
		DialContext: (&net.Dialer{
			Timeout:   1 * time.Second,
			KeepAlive: 1 * time.Second,
			DualStack: false,
		}).DialContext,
	}
	client := &http.Client{
		Transport: tr,
	}

	var resp *http.Response
	const delay = 1 * time.Second
	for i := 0; i != limit; i++ {
		resp, err = client.Do(req)
		if resp != nil {
			t.Log("GET: ", appURL, resp.StatusCode)
		}
		if err != nil {
			time.Sleep(delay)
			continue
		}
		if resp != nil && resp.StatusCode == http.StatusOK {
			break
		}
		time.Sleep(delay)
	}
	assert.NoError(t, err)

	return resp
}
func queryApp(t *testing.T, appURL string, limit int) {
	t.Helper()
	queryAppWithHostname(t, appURL, limit, "test.localhost")
}

func queryAppWithHostname(t *testing.T, appURL string, limit int, hostname string) {
	t.Helper()
	// Assert provider launches app in kind cluster

	req, err := http.NewRequest("GET", appURL, nil)
	require.NoError(t, err)
	req.Host = hostname // NOTE: cannot be inserted as a req.Header element, that is overwritten by this req.Host field.
	req.Header.Add("Cache-Control", "no-cache")
	req.Header.Add("Connection", "keep-alive")
	// Assert that the service is accessible. Unfortunately brittle single request.
	tr := &http.Transport{
		DisableKeepAlives: false,
	}
	client := &http.Client{
		Transport: tr,
	}

	// retry mechanism
	var resp *http.Response
	for i := 0; i < limit; i++ {
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
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	bytes, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(bytes), "The Future of The Cloud is Decentralized")
}

// appEnv asserts that there is an addressable docker container for KinD
func appEnv(t *testing.T) (string, string) {
	t.Helper()
	host := os.Getenv("KUBE_INGRESS_IP")
	require.NotEmpty(t, host)
	appPort := os.Getenv("KUBE_INGRESS_PORT")
	require.NotEmpty(t, appPort)
	return host, appPort
}

// this function is a gentle response to inappropriate approach of cli test utils
// send transaction may fail and calling cli routine won't know about it
func validateTxSuccessful(t testing.TB, cctx client.Context, data []byte) {
	t.Helper()

	var resp sdk.TxResponse

	err := jsonpb.Unmarshal(bytes.NewBuffer(data), &resp)
	require.NoError(t, err)

	res, err := authtx.QueryTx(cctx, resp.TxHash)
	require.NoError(t, err)

	require.Zero(t, res.Code, res)
}
