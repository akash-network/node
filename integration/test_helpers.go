// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clientkeys "github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/integration/cosmostests"
	dquery "github.com/ovrclk/akash/x/deployment/query"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
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
	_ = func() string {
		common.InitSDKConfig()
		return ""
	}()
)

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
	Ctx          context.Context
	BuildDir     string
	RootDir      string
	AkashdBinary string
	AkashBinary  string
	ChainID      string
	RPCAddr      string
	Port         string
	AkashdHome   string
	AkashHome    string
	P2PAddr      string
	T            *testing.T
}

// NewFixtures creates a new instance of Fixtures with many vars set
func NewFixtures(t *testing.T) *Fixtures {
	tmpDir, err := ioutil.TempDir("", "akash_integration_"+t.Name()+"_")
	require.NoError(t, err)

	// Prevent akashd errors on exit due to data saving behavior.
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
		T:            t,
		Ctx:          context.Background(),
		BuildDir:     buildDir,
		RootDir:      tmpDir,
		AkashdBinary: filepath.Join(buildDir, "akashd"),
		AkashBinary:  filepath.Join(buildDir, "akashctl"),
		AkashdHome:   filepath.Join(tmpDir, ".akashd"),
		AkashHome:    filepath.Join(tmpDir, ".akashctl"),
		RPCAddr:      servAddr,
		P2PAddr:      p2pAddr,
		Port:         port,
	}
}

// GenesisFile returns the path of the genesis file
func (f Fixtures) GenesisFile() string {
	return filepath.Join(f.AkashdHome, "config", "genesis.json")
}

// GenesisState returns the application's genesis state
func (f Fixtures) GenesisState() simapp.GenesisState {
	cdc := codec.New()
	genDoc, err := tmtypes.GenesisDocFromFile(f.GenesisFile())
	require.NoError(f.T, err)

	var appState simapp.GenesisState

	require.NoError(f.T, cdc.UnmarshalJSON(genDoc.AppState, &appState))

	return appState
}

// InitFixtures is called at the beginning of a test  and initializes a chain
// with 1 validator.
func InitFixtures(t *testing.T) (f *Fixtures) {
	f = NewFixtures(t)

	// reset test state
	f.UnsafeResetAll()

	// ensure keystore has foo and bar keys
	f.KeysDelete(keyFoo)
	f.KeysDelete(keyBar)
	f.KeysDelete(keyBaz)
	f.KeysAdd(keyFoo)
	f.KeysAdd(keyBar)
	f.KeysAdd(keyBaz)

	// ensure that CLI output is in JSON format
	f.CLIConfig("output", "json")

	// NOTE: AkashdInit sets the ChainID
	f.AkashdInit(keyFoo)

	f.CLIConfig("chain-id", f.ChainID)
	f.CLIConfig("broadcast-mode", "block")
	f.CLIConfig("trust-node", "true")

	// start an account with tokens
	f.AddGenesisAccount(f.KeyAddress(keyFoo), startCoins())
	f.AddGenesisAccount(f.KeyAddress(keyBar), startCoins())

	f.GenTx(keyFoo)
	f.CollectGenTxs()

	return f
}

// Cleanup is meant to be run at the end of a test to clean up an remaining test state
func (f *Fixtures) Cleanup(dirs ...string) {
	for _, d := range dirs {
		require.NoError(f.T, os.RemoveAll(d))
	}
	os.RemoveAll(f.RootDir)
}

// Flags returns the flags necessary for making most CLI calls
func (f *Fixtures) Flags() string {
	return fmt.Sprintf("--home=%s --node=%s", f.AkashHome, f.RPCAddr)
}

// KeyFlags returns the flags necessary for making most key CLI calls
func (f *Fixtures) KeyFlags() string {
	return fmt.Sprintf("--keyring-backend test")
}

//___________________________________________________________________________________
// akashd

// UnsafeResetAll is akashd unsafe-reset-all
func (f *Fixtures) UnsafeResetAll(flags ...string) {
	cmd := fmt.Sprintf("%s --home=%s unsafe-reset-all", f.AkashdBinary, f.AkashdHome)
	executeWrite(f.T, addFlags(cmd, flags))
	err := os.RemoveAll(filepath.Join(f.AkashdHome, "config", "gentx"))
	require.NoError(f.T, err)
}

// AkashdInit is akashd init
// NOTE: AkashdInit sets the ChainID for the Fixtures instance
func (f *Fixtures) AkashdInit(moniker string, flags ...string) {
	cmd := fmt.Sprintf("%s init -o --home=%s %s", f.AkashdBinary, f.AkashdHome, moniker)
	_, stderr := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var (
		chainID string
		initRes map[string]json.RawMessage
	)

	err := json.Unmarshal([]byte(stderr), &initRes)
	require.NoError(f.T, err)

	err = json.Unmarshal(initRes["chain_id"], &chainID)
	require.NoError(f.T, err)

	f.ChainID = chainID
}

// AddGenesisAccount is akashd add-genesis-account
func (f *Fixtures) AddGenesisAccount(address sdk.AccAddress, coins sdk.Coins, flags ...string) {
	cmd := fmt.Sprintf("%s add-genesis-account %s %s --home=%s %s", f.AkashdBinary, address,
		coins, f.AkashdHome, f.KeyFlags())
	executeWriteCheckErr(f.T, addFlags(cmd, flags))
}

// GenTx is akashd gentx
func (f *Fixtures) GenTx(name string, flags ...string) {
	cmd := fmt.Sprintf("%s gentx --name=%s --home=%s --home-client=%s %s", f.AkashdBinary, name, f.AkashdHome,
		f.AkashHome, f.KeyFlags())
	executeWriteCheckErr(f.T, addFlags(cmd, flags), "")
}

// CollectGenTxs is akashd collect-gentxs
func (f *Fixtures) CollectGenTxs(flags ...string) {
	cmd := fmt.Sprintf("%s collect-gentxs --home=%s", f.AkashdBinary, f.AkashdHome)
	executeWriteCheckErr(f.T, addFlags(cmd, flags), "")
}

// AkashdStart runs akashd start with the appropriate flags and returns a process
func (f *Fixtures) AkashdStart(flags ...string) *tests.Process {
	cmd := fmt.Sprintf("%s start --home=%s --rpc.laddr=%v --p2p.laddr=%v", f.AkashdBinary,
		f.AkashdHome, f.RPCAddr, f.P2PAddr)
	proc := cosmostests.GoExecuteT(f.T, addFlags(cmd, flags), []string{})
	cosmostests.WaitForTMStart(f.Port)
	cosmostests.WaitForNextNBlocksTM(2, f.Port)

	return proc
}

// ProviderStart launches the provider service
/*
Functional example from _run/kube
AKASH_DEPLOYMENT_INGRESS_STATIC_HOSTS="false" \
        ../../akashctl --home "./cache/client" --keyring-backend=test provider run \
                --from "provider" \
                --cluster-k8s \
                --gateway-listen-address "localhost:8080"
D[2020-08-03|16:56:26.423] reading kube config                          path=/home/ropes/.kube/config
I[2020-08-03|16:56:26.472] found leases                                 module=provider-cluster cmp=service num-active=0
*/
func (f *Fixtures) ProviderStart(id, provHost string, flags ...string) (*cosmostests.Process, string) {
	cmd := fmt.Sprintf("%s provider run --from=%s --cluster-k8s --gateway-listen-address=%s %s %s",
		f.AkashBinary,
		id,
		provHost,
		f.Flags(),
		f.KeyFlags(),
	)

	envvars := []string{"AKASH_DEPLOYMENT_INGRESS_STATIC_HOSTS=false"}
	proc := cosmostests.GoExecuteT(f.T, addFlags(cmd, flags), envvars)
	cosmostests.WaitForNextNBlocksTM(2, f.Port)
	//tests.WaitForStart(fmt.Sprintf("%s/status", provURL.String()))
	return proc, provHost
}

// SendManifest provides SDL to the Provider
func (f *Fixtures) SendManifest(lease types.Lease, sdlPath string, flags ...string) {
	leaseID := lease.ID()
	cmd := fmt.Sprintf("%s provider send-manifest --owner %s --dseq %v --gseq %v --oseq %v --provider %s %s %s",
		f.AkashBinary,
		leaseID.Owner.String(), leaseID.DSeq, leaseID.GSeq, leaseID.OSeq,
		leaseID.Provider.String(),
		sdlPath,
		f.Flags(),
	)

	_, stderr := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})
	assert.Empty(f.T, stderr, "sendmanifest stderr")
	cosmostests.WaitForNextNBlocksTM(2, f.Port)
}

// ValidateGenesis runs akashd validate-genesis
func (f *Fixtures) ValidateGenesis() {
	cmd := fmt.Sprintf("%s validate-genesis --home=%s", f.AkashdBinary, f.AkashdHome)
	executeWriteCheckErr(f.T, cmd)
}

//___________________________________________________________________________________
// akash config

// CLIConfig is akash config
func (f *Fixtures) CLIConfig(key, value string, flags ...string) {
	cmd := fmt.Sprintf("%s config --home=%s %s %s", f.AkashBinary, f.AkashHome, key, value)
	executeWriteCheckErr(f.T, addFlags(cmd, flags))
}

//___________________________________________________________________________________
// akash keys

// KeysAdd is akash keys add
func (f *Fixtures) KeysAdd(name string, flags ...string) {
	cmd := fmt.Sprintf("%s keys add --home=%s %s %s", f.AkashBinary, f.AkashHome, name, f.KeyFlags())
	executeWriteCheckErr(f.T, addFlags(cmd, flags), "y")
}

// KeysDelete is akash keys delete
func (f *Fixtures) KeysDelete(name string, flags ...string) {
	cmd := fmt.Sprintf("%s keys delete --home=%s %s %s", f.AkashBinary, f.AkashHome, name, f.KeyFlags())
	executeWrite(f.T, addFlags(cmd, append(append(flags, "-y"), "-f")))
}

// KeysAddRecover prepares akash keys add --recover
func (f *Fixtures) KeysAddRecover(name, mnemonic string, flags ...string) (exitSuccess bool, stdout, stderr string) {
	cmd := fmt.Sprintf("%s keys add --home=%s --recover %s %s", f.AkashBinary, f.AkashHome, name, f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags), mnemonic)
}

/*
// KeysShow is akash keys show
func (f *Fixtures) KeysShow(name string, flags ...string) keys.KeyOutput {
	cmd := fmt.Sprintf("%s keys show --home=%s %s -o json %s", f.AkashBinary, f.AkashHome, name, f.KeyFlags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var ko keys.KeyOutput

	err := clientkeys.UnmarshalJSON([]byte(out), &ko)
	require.NoError(f.T, err)

	return ko
}

// KeysList is akash keys list
func (f *Fixtures) KeysList(flags ...string) []keys.KeyOutput {
	cmd := fmt.Sprintf("%s keys list --home=%s -o json %s", f.AkashBinary, f.AkashHome, f.KeyFlags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var list []keys.KeyOutput

	err := clientkeys.UnmarshalJSON([]byte(out), &list)
	require.NoError(f.T, err)

	return list
}

// KeyAddress returns the SDK account address from the key
func (f *Fixtures) KeyAddress(name string) sdk.AccAddress {
	ko := f.KeysShow(name)
	accAddr, err := sdk.AccAddressFromBech32(ko.Address)
	require.NoError(f.T, err)

	return accAddr
}
*/

//___________________________________________________________________________________
// akash query account

// QueryAccount is akash query account
func (f *Fixtures) QueryAccount(address sdk.AccAddress, flags ...string) auth.BaseAccount {
	cmd := fmt.Sprintf("%s query account %s %v", f.AkashBinary, address, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string)

	var initRes map[string]json.RawMessage
	err := json.Unmarshal([]byte(out), &initRes)
	require.NoError(f.T, err, "out %v, err %v", out, err)

	value := initRes["value"]

	var acc auth.BaseAccount

	cdc := codec.New()
	codec.RegisterCrypto(cdc)
	err = cdc.UnmarshalJSON(value, &acc)
	require.NoError(f.T, err, "value %v, err %v", string(value), err)

	return acc
}

//___________________________________________________________________________________
// akash tx send/sign/broadcast

// TxSend is akash tx send
func (f *Fixtures) TxSend(from string, to sdk.AccAddress, amount sdk.Coin, flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx send %s %s %s %v %s", f.AkashBinary, from, to, amount, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

//___________________________________________________________________________________
// akash tx deployment

// TxCreateDeployment is akashctl create deployment
func (f *Fixtures) TxCreateDeployment(dfp string, flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx deployment create %s %v %s", f.AkashBinary, dfp, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

// TxUpdateDeployment is akashctl update deployment
func (f *Fixtures) TxUpdateDeployment(flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx deployment update %s %v %s", f.AkashBinary, deploymentV2FilePath, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

// TxCloseDeployment is akashctl close deployment
func (f *Fixtures) TxCloseDeployment(flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx deployment close %v %s", f.AkashBinary, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

//___________________________________________________________________________________
// akash query deployment

// QueryDeployments is akash query deployments
func (f *Fixtures) QueryDeployments(flags ...string) (dquery.Deployments, error) {
	cmd := fmt.Sprintf("%s query deployment list %v", f.AkashBinary, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var deployments dquery.Deployments

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &deployments)

	return deployments, err
}

// QueryDeployment is akash query deployment
func (f *Fixtures) QueryDeployment(depID dtypes.DeploymentID, flags ...string) dquery.Deployment {
	cmd := fmt.Sprintf("%s query deployment get --owner %s --dseq %v %v", f.AkashBinary,
		depID.Owner.String(), depID.DSeq, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var deployment dquery.Deployment

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &deployment)
	require.NoError(f.T, err, "out %v\n, err %v", out, err)

	return deployment
}

//___________________________________________________________________________________
// akash tx market

// TxCreateBid is akash create bid
func (f *Fixtures) TxCreateBid(oid mtypes.OrderID, price sdk.Coin, flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx market bid-create --owner %s --dseq %v --gseq %v --oseq %v --price %s %v %s",
		f.AkashBinary, oid.Owner.String(), oid.DSeq, oid.GSeq, oid.OSeq, price, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

// TxCloseBid is akash close bid
func (f *Fixtures) TxCloseBid(oid mtypes.OrderID, flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx market bid-close --owner %s --dseq %v --gseq %v --oseq %v %v %s",
		f.AkashBinary, oid.Owner.String(), oid.DSeq, oid.GSeq, oid.OSeq, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

// TxCloseOrder is akash close order
func (f *Fixtures) TxCloseOrder(oid mtypes.OrderID, flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx market order-close --owner %s --dseq %v --gseq %v --oseq %v %v %s",
		f.AkashBinary, oid.Owner.String(), oid.DSeq, oid.GSeq, oid.OSeq, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

//___________________________________________________________________________________
// akash query market

// QueryOrders is akash query orders
func (f *Fixtures) QueryOrders(flags ...string) ([]mtypes.Order, error) {
	cmd := fmt.Sprintf("%s query market order list %v", f.AkashBinary, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string)

	var orders []mtypes.Order

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &orders)

	return orders, err
}

// QueryOrder is akash query order
func (f *Fixtures) QueryOrder(orderID mtypes.OrderID, flags ...string) mtypes.Order {
	cmd := fmt.Sprintf("%s query market order get --owner %s --dseq %v --gseq %v --oseq %v %v", f.AkashBinary,
		orderID.Owner.String(), orderID.DSeq, orderID.GSeq, orderID.OSeq, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var order mtypes.Order

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &order)
	require.NoError(f.T, err, "out %v\n, err %v", out, err)

	return order
}

// QueryBids is akash query bids
func (f *Fixtures) QueryBids(flags ...string) ([]mtypes.Bid, error) {
	cmd := fmt.Sprintf("%s query market bid list %v", f.AkashBinary, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string)

	var bids []mtypes.Bid

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &bids)

	return bids, err
}

// QueryBid is akash query bid
func (f *Fixtures) QueryBid(bidID mtypes.BidID, flags ...string) mtypes.Bid {
	cmd := fmt.Sprintf("%s query market bid get --owner %s --dseq %v", f.AkashBinary,
		bidID.Owner.String(), bidID.DSeq)
	cmd += fmt.Sprintf(" --gseq %v --oseq %v --provider %s %v", bidID.GSeq, bidID.OSeq,
		bidID.Provider.String(), f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var bid mtypes.Bid

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &bid)
	require.NoError(f.T, err, "out %v\n, err %v", out, err)

	return bid
}

// QueryLeases is akash query leases
func (f *Fixtures) QueryLeases(flags ...string) ([]mtypes.Lease, error) {
	cmd := fmt.Sprintf("%s query market lease list %v", f.AkashBinary, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var leases []mtypes.Lease

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &leases)

	return leases, err
}

// QueryLease is akash query lease
func (f *Fixtures) QueryLease(leaseID mtypes.LeaseID, flags ...string) mtypes.Lease {
	cmd := fmt.Sprintf("%s query market lease get --owner %s --dseq %v", f.AkashBinary,
		leaseID.Owner.String(), leaseID.DSeq)
	cmd += fmt.Sprintf(" --gseq %v --oseq %v --provider %s %v", leaseID.GSeq, leaseID.OSeq,
		leaseID.Provider.String(), f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var lease mtypes.Lease

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &lease)
	require.NoError(f.T, err, "out %v\n, err %v", out, err)

	return lease
}

//___________________________________________________________________________________
// akash tx provider

// TxCreateProvider is akash create provider
func (f *Fixtures) TxCreateProvider(flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx provider create %s %v %s", f.AkashBinary, providerFilePath, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

// TxCreateProvider is akash create provider
func (f *Fixtures) TxCreateProviderFromFile(path string, flags ...string) (bool, string, string) {
	cmd := fmt.Sprintf("%s tx provider create %s %v %s", f.AkashBinary, path, f.Flags(), f.KeyFlags())
	return executeWriteRetStdStreams(f.T, addFlags(cmd, flags))
}

//___________________________________________________________________________________
// akash query provider

// QueryProviders is akash query providers
func (f *Fixtures) QueryProviders(flags ...string) []ptypes.Provider {
	cmd := fmt.Sprintf("%s query provider list %v", f.AkashBinary, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var providers []ptypes.Provider

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &providers)
	require.NoError(f.T, err, "out %v\n, err %v", out, err)

	return providers
}

// QueryProvider is akash query provider
func (f *Fixtures) QueryProvider(owner string, flags ...string) ptypes.Provider {
	cmd := fmt.Sprintf("%s query provider get %s %v", f.AkashBinary, owner, f.Flags())
	out, _ := cosmostests.ExecuteT(f.T, addFlags(cmd, flags), "", []string{})

	var provider ptypes.Provider

	cdc := app.MakeCodec()
	err := cdc.UnmarshalJSON([]byte(out), &provider)
	require.NoError(f.T, err, "out %v\n, err %v", out, err)

	return provider
}

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

//___________________________________________________________________________________
// utils

func addFlags(cmd string, flags []string) string {
	for _, f := range flags {
		cmd += " " + f
	}

	return strings.TrimSpace(cmd)
}
