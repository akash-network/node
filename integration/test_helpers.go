// +build integration

package integration

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovrclk/akash/x/market/types"
)

const (
	denom                = "akash"
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

// utils
func addFlags(cmd string, flags []string) string {
	for _, f := range flags {
		cmd += " " + f
	}

	return strings.TrimSpace(cmd)
}

// SendManifest provides SDL to the Provider
func SendManifest(lease types.Lease, sdlPath string, flags ...string) {
	leaseID := lease.ID()
	args := []string{
		fmt.Sprintf("--owner=%s", leaseID.Owner.String()),
		fmt.Sprintf("--dseq=%v", leaseID.DSeq),
		fmt.Sprintf("--gseq=%v", leaseID.GSeq),
		fmt.Sprintf("--oseq=%v", leaseID.OSeq),
		fmt.Sprintf("--provider=%s", leaseID.Provider.String()),
		sdlPath,
	}
	args = append(args, flags...)
	// Define command arguments
	/*
		cmd := fmt.Sprintf("%s provider send-manifest --owner %s --dseq %v --gseq %v --oseq %v --provider %s %s %s",
			f.AkashBinary,
			leaseID.Owner.String(), leaseID.DSeq, leaseID.GSeq, leaseID.OSeq,
			leaseID.Provider.String(),
			sdlPath,
			f.Flags(),
		)
	*/

	// TODO: call send-manifest cobra command
}

// Assert provider launches app in kind cluster
func queryApp(t *testing.T, appURL string) {
	req, err := http.NewRequest("GET", appURL, nil)
	require.NoError(t, err)
	req.Host = "hello.localhost" // NOTE: cannot be inserted as a req.Header element, that is overwritten by this req.Host field.
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
