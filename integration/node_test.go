package integration

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/ovrclk/akash/app"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/stretchr/testify/require"
)

func TestAddGenesisAccount(t *testing.T) {
	t.Parallel()
	f := NewFixtures(t)

	// Reset testing path
	f.UnsafeResetAll()

	// Initialize keys
	f.KeysDelete(keyFoo)
	f.KeysDelete(keyBar)
	f.KeysDelete(keyBaz)
	f.KeysAdd(keyFoo)
	f.KeysAdd(keyBar)
	f.KeysAdd(keyBaz)

	// Configure json output
	f.CLIConfig("output", "json")

	// Run init
	f.AkashdInit(keyFoo)

	// Add account to genesis.json
	bazCoins := sdk.Coins{
		sdk.NewInt64Coin("acoin", 1000000),
		sdk.NewInt64Coin("bcoin", 1000000),
	}

	f.AddGenesisAccount(f.KeyAddress(keyFoo), startCoins())
	f.AddGenesisAccount(f.KeyAddress(keyBar), bazCoins)
	genesisState := f.GenesisState()

	cdc := app.MakeCodec()
	accounts := auth.GetGenesisStateFromAppState(cdc, genesisState)
	// t.Logf("Accounts:%v\n", accounts)
	require.Equal(t, accounts.Accounts[0].GetAddress(), f.KeyAddress(keyFoo))
	require.Equal(t, accounts.Accounts[1].GetAddress(), f.KeyAddress(keyBar))
	require.True(t, accounts.Accounts[0].GetCoins().IsEqual(startCoins()))
	require.True(t, accounts.Accounts[1].GetCoins().IsEqual(bazCoins))

	// Cleanup testing directories
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
	f.AddGenesisAccount(f.KeyAddress(keyFoo), startCoins())

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
