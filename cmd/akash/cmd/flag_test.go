package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
	testutilcli "github.com/ovrclk/akash/testutil/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	tmcli "github.com/tendermint/tendermint/libs/cli"
)

// TestContextFlags tests that all the flags which are set in client.Context are parsed correctly.
// This test has been added because recently the --home flag broke with cosmos-sdk@v0.43.0 upgrade.
func TestContextFlags(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test-akash-home")
	require.NoError(t, err)

	// expected flag values
	output := "test-output" // default = "json"
	home := tmpDir
	dryRun := true // default = false
	keyringDir := "/test/keyring/dir"
	chainID := "test-chain-id"
	node := "http://test-host:8080" // default = "tcp://localhost:26657"
	height := int64(20)             // default = 0
	useLedger := true               // default = false
	generateOnly := true            // default = false
	offline := true                 // default = false
	broadcastMode := "block"        // default = "sync"
	skipConfirmation := true        // default = false
	signMode := "direct"
	feeAccount := testutil.AccAddress(t).String()
	fromAddr := testutil.AccAddress(t)

	// helper functions to check flag correctness
	checkPersistentCommandFlags := func(clientCtx client.Context) {
		require.Equal(t, output, clientCtx.OutputFormat)
		require.Equal(t, home, clientCtx.HomeDir)
		require.Equal(t, dryRun, clientCtx.Simulate)
		require.Equal(t, keyringDir, clientCtx.KeyringDir)
		require.Equal(t, chainID, clientCtx.ChainID)
		require.Equal(t, node, clientCtx.NodeURI)
	}
	checkQueryOnlyFlags := func(clientCtx client.Context) {
		require.Equal(t, height, clientCtx.Height)
		require.Equal(t, useLedger, clientCtx.UseLedger)
	}
	checkTxOnlyFlags := func(clientCtx client.Context) {
		require.Equal(t, generateOnly, clientCtx.GenerateOnly)
		require.Equal(t, offline, clientCtx.Offline)
		require.Equal(t, broadcastMode, clientCtx.BroadcastMode)
		require.Equal(t, skipConfirmation, clientCtx.SkipConfirm)
		require.Equal(t, signMode, clientCtx.SignModeStr)
		require.Equal(t, feeAccount, clientCtx.FeeGranter.String())
		require.Equal(t, fromAddr, clientCtx.FromAddress)
		require.Equal(t, fromAddr.String(), clientCtx.From)
	}

	// test command
	cmd := &cobra.Command{
		Use:               "test",
		PersistentPreRunE: persistentPreRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			// check that the PersistentCommandFlags have been set correctly based on
			// PersistentPreRunE
			clientCtx := client.GetClientContextFromCmd(cmd)
			checkPersistentCommandFlags(clientCtx)

			// check that query flags have been set correctly, in addition to PersistentCommandFlags
			clientCtx, err = client.GetClientQueryContext(cmd)
			require.NoError(t, err)
			checkQueryOnlyFlags(clientCtx)
			checkPersistentCommandFlags(clientCtx)

			// check that tx flags have been set correctly, in addition to PersistentCommandFlags
			clientCtx, err = client.GetClientTxContext(cmd)
			require.NoError(t, err)
			checkTxOnlyFlags(clientCtx)
			checkPersistentCommandFlags(clientCtx)

			return nil
		},
	}
	cmd.PersistentFlags().String(flags.FlagHome, app.DefaultHome, "The application home directory")
	cmd.PersistentFlags().String(flags.FlagChainID, "", "The network chain ID")
	cmd.Flags().Int64(flags.FlagHeight, 0, "Use a specific height to query state at (this can error if the node is pruning state)")
	flags.AddTxFlagsToCmd(cmd)

	// run the test command with expected flag values
	_, err = testutilcli.ExecTestCLICmd(
		client.Context{},
		cmd,
		fmt.Sprintf("--%s=%s", tmcli.OutputFlag, output),
		fmt.Sprintf("--%s=%s", flags.FlagHome, home),
		fmt.Sprintf("--%s=%v", flags.FlagDryRun, dryRun),
		fmt.Sprintf("--%s=%s", flags.FlagKeyringDir, keyringDir),
		fmt.Sprintf("--%s=%s", flags.FlagChainID, chainID),
		fmt.Sprintf("--%s=%s", flags.FlagNode, node),
		fmt.Sprintf("--%s=%d", flags.FlagHeight, height),
		fmt.Sprintf("--%s=%v", flags.FlagUseLedger, useLedger),
		fmt.Sprintf("--%s=%v", flags.FlagGenerateOnly, generateOnly),
		fmt.Sprintf("--%s=%v", flags.FlagOffline, offline),
		fmt.Sprintf("--%s=%s", flags.FlagBroadcastMode, broadcastMode),
		fmt.Sprintf("--%s=%v", flags.FlagSkipConfirmation, skipConfirmation),
		fmt.Sprintf("--%s=%s", flags.FlagSignMode, signMode),
		fmt.Sprintf("--%s=%s", flags.FlagFeeAccount, feeAccount),
		fmt.Sprintf("--%s=%s", flags.FlagFrom, fromAddr.String()),
	)
	require.NoError(t, err)

	// cleanup
	require.NoError(t, os.RemoveAll(tmpDir))
}
