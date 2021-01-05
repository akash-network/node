// +build integration

package integration

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	denom                = "uakt"
	denomStartValue      = 150
	keyFoo               = "foo"
	keyBar               = "bar"
	keyBaz               = "baz"
	fooStartValue        = 1000
	feeDenom             = "uakt"
	feeStartValue        = 1000000
	deploymentFilePath   = "./../x/deployment/testdata/deployment.yaml"
	deploymentV2FilePath = "./../x/deployment/testdata/deployment-v2.yaml"
	deploymentOvrclkApp  = "./../_run/kube/deployment.yaml"
	providerFilePath     = "./../x/provider/testdata/provider.yaml"
	providerTemplate     = `host: %s
attributes:
  - key: region
    value: us-west
  - key: moniker
    value: akash
`
)

// newAkashCoin
func newAkashCoin(amt int64) sdk.Coin {
	return sdk.NewInt64Coin(denom, amt)
}

//___________________________________________________________________________________
// utils
func addFlags(cmd string, flags []string) string {
	for _, f := range flags {
		cmd += " " + f
	}

	return strings.TrimSpace(cmd)
}

func queryAppWithRetries(t *testing.T, appURL string, appHost string, limit int) *http.Response {
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
	queryAppWithHostname(t, appURL, limit, "test.localhost")
}


func queryAppWithHostname(t *testing.T, appURL string, limit int, hostname string) {
// Assert provider launches app in kind cluster

	req, err := http.NewRequest("GET", appURL, nil)
	require.NoError(t, err)
	req.Host = hostname // NOTE: cannot be inserted as a req.Header element, that is overwritten by this req.Host field.
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
	host := os.Getenv("KUBE_INGRESS_IP")
	require.NotEmpty(t, host)
	appPort := os.Getenv("KUBE_INGRESS_PORT")
	require.NotEmpty(t, appPort)
	return host, appPort
}
