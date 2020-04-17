package integration

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"testing"

	"github.com/cosmos/cosmos-sdk/tests"
	sdk "github.com/cosmos/cosmos-sdk/types"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/stretchr/testify/require"
)

func TestDeployment(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start gaiad server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	// Save key addresses for later use
	fooAddr := f.KeyAddress(keyFoo)

	fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

	// Create deployment
	f.TxCreateDeployment(fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments := f.QueryDeployments()
	require.Len(t, deployments, 1, "Deployment Create Failed")
	require.Equal(t, fooAddr.String(), deployments[0].Deployment.DeploymentID.Owner.String())

	// test query deployment
	createdDep := deployments[0]
	deployment := f.QueryDeployment(createdDep.Deployment.DeploymentID)
	require.Equal(t, createdDep, deployment)

	// test query deployments with filters
	deployments = f.QueryDeployments("--state=closed")
	require.Len(t, deployments, 0)

	// Close deployment
	f.TxCloseDeployment(fmt.Sprintf("--from=%s --dseq=%v", keyFoo, createdDep.Deployment.DeploymentID.DSeq), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments = f.QueryDeployments()
	require.Len(t, deployments, 1)
	require.Equal(t, uint8(1), uint8(deployments[0].Deployment.State), "Deployment Close Failed")

	// test query deployments with state filter closed
	deployments = f.QueryDeployments("--state=closed")
	require.Len(t, deployments, 1)

	f.Cleanup()
}

func TestMarket(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start gaiad server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	// Save key addresses for later use
	fooAddr := f.KeyAddress(keyFoo)
	barAddr := f.KeyAddress(keyBar)

	fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

	// Create deployment
	f.TxCreateDeployment(fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query deployments
	deployments := f.QueryDeployments()
	require.Len(t, deployments, 1, "Deployment Creation Failed in TestMarket")
	require.Equal(t, fooAddr.String(), deployments[0].Deployment.DeploymentID.Owner.String())

	// test query orders
	orders := f.QueryOrders()
	require.Len(t, orders, 1)
	require.Equal(t, fooAddr.String(), orders[0].OrderID.Owner.String())

	// test query order
	createdOrder := orders[0]
	order := f.QueryOrder(createdOrder.OrderID)
	require.Equal(t, createdOrder, order)

	// Create provider
	f.TxCreateProvider(fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query providers
	providers := f.QueryProviders()
	require.Len(t, providers, 1, "Provider Creation Failed in TestMarket")
	require.Equal(t, barAddr.String(), providers[0].Owner.String())

	// Create Bid
	f.TxCreateBid(createdOrder.OrderID, sdk.NewInt64Coin(denom, 20), fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(3, f.Port)

	// test query bids
	bids := f.QueryBids()
	require.Len(t, bids, 1, "Creating bid failed")
	require.Equal(t, barAddr.String(), bids[0].Provider.String())

	// test query bid
	createdBid := bids[0]
	bid := f.QueryBid(createdBid.BidID)
	require.Equal(t, createdBid, bid)

	// test query leases
	leases := f.QueryLeases()
	require.Len(t, leases, 1)

	// test query order
	createdLease := leases[0]
	lease := f.QueryLease(createdLease.LeaseID)
	require.Equal(t, createdLease, lease)

	// Close Bid
	f.TxCloseBid(createdOrder.OrderID, fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(2, f.Port)

	// test query bids with filter
	closedBids := f.QueryBids("--state=closed")
	require.Len(t, closedBids, 1, "Closing bid failed")
	require.Equal(t, barAddr.String(), closedBids[0].Provider.String())

	// test query leases with filter
	closedLeases := f.QueryLeases("--state=closed")
	require.Len(t, closedLeases, 1)

	// test query orders with filter state open
	openedOrders := f.QueryOrders("--state=open")
	require.Len(t, openedOrders, 1)

	// Creating bid again for new order
	f.TxCreateBid(openedOrders[0].OrderID, sdk.NewInt64Coin(denom, 20), fmt.Sprintf("--from=%s", keyBar), "-y")
	tests.WaitForNextNBlocksTM(3, f.Port)

	// test query bids
	matchedBids := f.QueryBids("--state=matched")
	require.Len(t, matchedBids, 1, "Creating bid failed second time")

	// Close Order
	f.TxCloseOrder(openedOrders[0].OrderID, fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query orders with filter state closed
	closedOrders := f.QueryOrders("--state=closed")
	require.Len(t, closedOrders, 2, "Closing Order failed")

	f.Cleanup()
}

func TestProvider(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start gaiad server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	// Save key addresses for later use
	fooAddr := f.KeyAddress(keyFoo)

	fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

	// Create provider
	f.TxCreateProvider(fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query providers
	providers := f.QueryProviders()
	require.Len(t, providers, 1, "Creating provider failed")
	require.Equal(t, fooAddr.String(), providers[0].Owner.String())

	// test query provider
	createdProvider := providers[0]
	provider := f.QueryProvider(createdProvider.Owner)
	require.Equal(t, createdProvider, provider)

	f.Cleanup()
}

func TestGaiaCLISend(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start gaiad server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	// Save key addresses for later use
	fooAddr := f.KeyAddress(keyFoo)
	bazAddr := f.KeyAddress(keyBaz)

	fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

	// Send some tokens from one account to the other
	sendTokens := sdk.TokensFromConsensusPower(10)
	f.TxSend(keyFoo, bazAddr, sdk.NewCoin(denom, sendTokens), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// Ensure account balances match expected
	barAcc := f.QueryAccount(bazAddr)
	require.Equal(t, sendTokens, barAcc.GetCoins().AmountOf(denom))

	fooAcc = f.QueryAccount(fooAddr)
	require.Equal(t, startTokens.Sub(sendTokens), fooAcc.GetCoins().AmountOf(denom))

	f.Cleanup()
}

func TestAkashConfig(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)
	node := fmt.Sprintf("%s:%s", f.RPCAddr, f.Port)

	// Set available configuration options
	f.CLIConfig("broadcast-mode", "block")
	f.CLIConfig("node", node)
	f.CLIConfig("output", "text")
	f.CLIConfig("trust-node", "true")
	f.CLIConfig("chain-id", f.ChainID)
	f.CLIConfig("trace", "false")
	f.CLIConfig("indent", "true")

	config, err := ioutil.ReadFile(path.Join(f.AkashHome, "config", "config.toml"))
	require.NoError(t, err)

	expectedConfig := fmt.Sprintf(`broadcast-mode = "block"
chain-id = "%s"
indent = true
node = "%s"
output = "text"
trace = false
trust-node = true
`, f.ChainID, node)
	require.Equal(t, expectedConfig, string(config))

	f.Cleanup()
}

func TestAkashdCollectGentxs(t *testing.T) {
	t.Parallel()

	var customMaxBytes, customMaxGas int64 = 99999999, 1234567

	f := NewFixtures(t)

	// Initialise temporary directories
	gentxDir, err := ioutil.TempDir("", "")
	gentxDoc := filepath.Join(gentxDir, "gentx.json")

	require.NoError(t, err)

	// Reset testing path
	f.UnsafeResetAll()

	// Initialize keys
	f.KeysAdd(keyFoo)

	// Configure json output
	f.CLIConfig("output", "json")

	// Run init
	f.AkashdInit(keyFoo)

	// Customise genesis.json

	genFile := f.GenesisFile()
	genDoc, err := tmtypes.GenesisDocFromFile(genFile)
	require.NoError(t, err)

	genDoc.ConsensusParams.Block.MaxBytes = customMaxBytes
	genDoc.ConsensusParams.Block.MaxGas = customMaxGas
	_ = genDoc.SaveAs(genFile)

	// Add account to genesis.json
	f.AddGenesisAccount(f.KeyAddress(keyFoo), startCoins)

	// Write gentx file
	f.GenTx(keyFoo, fmt.Sprintf("--output-document=%s", gentxDoc))

	// Collect gentxs from a custom directory
	f.CollectGenTxs(fmt.Sprintf("--gentx-dir=%s", gentxDir))

	genDoc, err = tmtypes.GenesisDocFromFile(genFile)
	require.NoError(t, err)
	require.Equal(t, genDoc.ConsensusParams.Block.MaxBytes, customMaxBytes)
	require.Equal(t, genDoc.ConsensusParams.Block.MaxGas, customMaxGas)

	f.Cleanup(gentxDir)
}

func TestValidateGenesis(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start akashd server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	f.ValidateGenesis()

	// Cleanup testing directories
	f.Cleanup()
}
