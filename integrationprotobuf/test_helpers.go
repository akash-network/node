// +build integration

package integrationprotobuf

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/server"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	denom                = "akash"
	denomStartValue      = 150
	keyFoo               = "foo"
	keyBar               = "bar"
	keyBaz               = "baz"
	fooDenom             = "footoken"
	fooStartValue        = 1000
	feeDenom             = "stake"
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

var (
	fooAddr = authtypes.NewEmptyModuleAccount(keyFoo)
	barAddr = authtypes.NewEmptyModuleAccount(keyBar)
)

// newAkashCoin
func newAkashCoin(amt int64) sdk.Coin {
	return sdk.NewInt64Coin(denom, amt)
}

func startCoins() sdk.Coins {
	return sdk.NewCoins(
		sdk.NewCoin(feeDenom, sdk.TokensFromConsensusPower(feeStartValue)),
		sdk.NewCoin(fooDenom, sdk.TokensFromConsensusPower(fooStartValue)),
		sdk.NewCoin(denom, sdk.TokensFromConsensusPower(denomStartValue)),
	)
}

//___________________________________________________________________________________
// Fixtures

// Fixtures is used to setup the testing environment
type Fixtures struct {
	Ctx context.Context

	BuildDir    string
	RootDir     string
	AkashBinary string
	ChainID     string
	RPCAddr     string
	Port        string
	AkashHome   string
	P2PAddr     string
	T           *testing.T
}

// NewFixtures creates a new instance of Fixtures with many vars set
func NewFixtures(t *testing.T) *Fixtures {

	tmpDir, err := ioutil.TempDir("", "akash_integration_"+t.Name()+"_")
	require.NoError(t, err)

	// Prevent akash errors on exit due to data saving behavior.
	tmpStat, err := os.Lstat(tmpDir)
	require.NoError(t, err)
	err = os.MkdirAll(fmt.Sprintf("%s/.akashd/data/cs.wal", tmpDir), tmpStat.Mode())
	require.NoError(t, err)

	servAddr, port, err := server.FreeTCPAddr()
	require.NoError(t, err)

	p2pAddr, _, err := server.FreeTCPAddr()
	require.NoError(t, err)

	buildDir := os.Getenv("BUILDDIR")
	if buildDir == "" {
		buildDir, err = filepath.Abs("../_build/")
		require.NoError(t, err)
	}

	return &Fixtures{
		T:           t,
		Ctx:         context.Background(),
		BuildDir:    buildDir,
		RootDir:     tmpDir,
		AkashBinary: filepath.Join(buildDir, "akash"),
		AkashHome:   filepath.Join(tmpDir, ".akash"),
		RPCAddr:     servAddr,
		P2PAddr:     p2pAddr,
		Port:        port,
	}
}

/*
//___________________________________________________________________________________
// executors

func executeWriteCheckErr(t *testing.T, cmdStr string, writes ...string) {
	require.True(t, executeWrite(t, cmdStr, writes...))
}

func executeWrite(t *testing.T, cmdStr string, writes ...string) (exitSuccess bool) {
	exitSuccess, _, _ = executeWriteRetStdStreams(t, cmdStr, writes...)
	return
}

func executeWriteRetStdStreams(t *testing.T, cmdStr string, writes ...string) (bool, string, string) {
	proc := cosmostests.GoExecuteT(t, cmdStr, []string{})

	// Enables use of interactive commands
	for _, write := range writes {
		_, err := proc.StdinPipe.Write([]byte(write + "\n"))
		require.NoError(t, err)
	}

	// Read both stdout and stderr from the process
	stdout, stderr, err := proc.ReadAll()
	if err != nil {
		fmt.Println("Err on proc.ReadAll()", err, cmdStr)
	}

	// // Log output.
	// if len(stdout) > 0 {
	// 	t.Log("Stdout:", string(stdout))
	// }

	// if len(stderr) > 0 {
	// 	t.Log("Stderr:", string(stderr))
	// }

	// Wait for process to exit
	proc.Wait()

	// Return succes, stdout, stderr
	return proc.ExitState.Success(), string(stdout), string(stderr)
}
*/

//___________________________________________________________________________________
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
