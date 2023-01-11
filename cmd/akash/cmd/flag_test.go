package cmd

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	tmcli "github.com/tendermint/tendermint/libs/cli"

	"github.com/akash-network/node/app"
	"github.com/akash-network/node/testutil"
	testutilcli "github.com/akash-network/node/testutil/cli"
)

// TestContextFlags tests that all the flags which are set in client.Context are parsed correctly.
// This test has been added because recently the --home flag broke with cosmos-sdk@v0.43.0 upgrade.
func TestContextFlags(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-akash-home")
	require.NoError(t, err)

	// expected flag values
	expectedFlagValues := map[string]interface{}{
		tmcli.OutputFlag:           "test-output", // default = "json"
		flags.FlagHome:             tmpDir,
		flags.FlagDryRun:           true, // default = false
		flags.FlagKeyringDir:       "/test/keyring/dir",
		flags.FlagChainID:          "test-chain-id",
		flags.FlagNode:             "http://test-host:8080", // default = "tcp://localhost:26657"
		flags.FlagHeight:           int64(20),               // default = 0
		flags.FlagUseLedger:        true,                    // default = false
		flags.FlagGenerateOnly:     true,                    // default = false
		flags.FlagOffline:          true,                    // default = false
		flags.FlagBroadcastMode:    "async",                 // default = "sync"
		flags.FlagSkipConfirmation: true,                    // default = false
		flags.FlagSignMode:         "direct",
		flags.FlagFeeAccount:       testutil.AccAddress(t).String(),
		flags.FlagFrom:             testutil.AccAddress(t).String(),
	}

	tCases := []struct {
		flag           string
		ctxFieldGetter func(ctx client.Context) interface{}
		isQueryOnly    bool
		isTxOnly       bool
	}{
		{
			flag: tmcli.OutputFlag,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.OutputFormat
			},
		},
		{
			flag: flags.FlagHome,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.HomeDir
			},
		},
		{
			flag: flags.FlagDryRun,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.Simulate
			},
		},
		{
			flag: flags.FlagKeyringDir,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.KeyringDir
			},
		},
		{
			flag: flags.FlagChainID,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.ChainID
			},
		},
		{
			flag: flags.FlagNode,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.NodeURI
			},
		},
		{
			flag: flags.FlagHeight,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.Height
			},
			isQueryOnly: true,
		},
		{
			flag: flags.FlagUseLedger,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.UseLedger
			},
			isQueryOnly: true,
		},
		{
			flag: flags.FlagGenerateOnly,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.GenerateOnly
			},
			isTxOnly: true,
		},
		{
			flag: flags.FlagOffline,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.Offline
			},
			isTxOnly: true,
		},
		{
			flag: flags.FlagBroadcastMode,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.BroadcastMode
			},
			isTxOnly: true,
		},
		{
			flag: flags.FlagSkipConfirmation,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.SkipConfirm
			},
			isTxOnly: true,
		},
		{
			flag: flags.FlagSignMode,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.SignModeStr
			},
			isTxOnly: true,
		},
		{
			flag: flags.FlagFeeAccount,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.FeeGranter.String()
			},
			isTxOnly: true,
		},
		{
			flag: flags.FlagFrom,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				require.Equal(t, ctx.From, ctx.FromAddress.String())
				return ctx.From
			},
			isTxOnly: true,
		},
	}

	// test command
	cmd := &cobra.Command{
		Use:               "test",
		PersistentPreRunE: GetPersistentPreRunE(app.MakeEncodingConfig(), []string{"AKASH"}),
	}
	cmd.PersistentFlags().String(flags.FlagHome, app.DefaultHome, "The application home directory")
	cmd.PersistentFlags().String(flags.FlagChainID, "", "The network chain ID")
	cmd.Flags().Int64(flags.FlagHeight, 0, "Use a specific height to query state at (this can error if the node is pruning state)")
	flags.AddTxFlagsToCmd(cmd)

	// test runner
	for _, tCase := range tCases {
		testCase := tCase
		t.Run(testCase.flag, func(t *testing.T) {
			// set the run func
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				var clientCtx client.Context

				// prepare context
				switch {
				case testCase.isQueryOnly:
					clientCtx, err = client.GetClientQueryContext(cmd)
					require.NoError(t, err)
				case testCase.isTxOnly:
					clientCtx, err = client.GetClientTxContext(cmd)
					require.NoError(t, err)
				default:
					clientCtx = client.GetClientContextFromCmd(cmd)
				}

				// check that we got the expected flag value in context
				require.Equal(t, expectedFlagValues[testCase.flag], testCase.ctxFieldGetter(clientCtx))

				return nil
			}

			// run the test command with expected flag value
			_, err = testutilcli.ExecTestCLICmd(
				context.Background(),
				client.Context{},
				cmd,
				fmt.Sprintf("--%s=%v", testCase.flag, expectedFlagValues[testCase.flag]),
			)
			require.NoError(t, err)
		})
	}

	// cleanup
	require.NoError(t, os.RemoveAll(tmpDir))
}
