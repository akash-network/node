package cmd

import (
	"context"
	"fmt"
	"os"
	"testing"

	tmcli "github.com/cometbft/cometbft/libs/cli"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cflags "pkg.akt.dev/go/cli/flags"

	"pkg.akt.dev/akashd/app"
	"pkg.akt.dev/akashd/testutil"
	testutilcli "pkg.akt.dev/akashd/testutil/cli"
)

// TestContextFlags tests that all the flags which are set in client.Context are parsed correctly.
// This test has been added because recently the --home flag broke with cosmos-sdk@v0.43.0 upgrade.
func TestContextFlags(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-akash-home")
	require.NoError(t, err)

	// expected flag values
	expectedFlagValues := map[string]interface{}{
		tmcli.OutputFlag:            "test-output", // default = "json"
		cflags.FlagHome:             tmpDir,
		cflags.FlagDryRun:           true, // default = false
		cflags.FlagKeyringDir:       "/test/keyring/dir",
		cflags.FlagChainID:          "test-chain-id",
		cflags.FlagNode:             "http://test-host:8080", // default = "tcp://localhost:26657"
		cflags.FlagHeight:           int64(20),               // default = 0
		cflags.FlagUseLedger:        true,                    // default = false
		cflags.FlagGenerateOnly:     true,                    // default = false
		cflags.FlagOffline:          true,                    // default = false
		cflags.FlagBroadcastMode:    "async",                 // default = "sync"
		cflags.FlagSkipConfirmation: true,                    // default = false
		cflags.FlagSignMode:         "direct",
		// cli.FlagFeeAccount:       testutil.AccAddress(t).String(),
		cflags.FlagFrom: testutil.AccAddress(t).String(),
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
			flag: cflags.FlagHome,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.HomeDir
			},
		},
		{
			flag: cflags.FlagDryRun,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.Simulate
			},
		},
		{
			flag: cflags.FlagKeyringDir,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.KeyringDir
			},
		},
		{
			flag: cflags.FlagChainID,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.ChainID
			},
		},
		{
			flag: cflags.FlagNode,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.NodeURI
			},
		},
		{
			flag: cflags.FlagHeight,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.Height
			},
			isQueryOnly: true,
		},
		{
			flag: cflags.FlagUseLedger,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.UseLedger
			},
			isQueryOnly: true,
		},
		{
			flag: cflags.FlagGenerateOnly,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.GenerateOnly
			},
			isTxOnly: true,
		},
		{
			flag: cflags.FlagOffline,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.Offline
			},
			isTxOnly: true,
		},
		{
			flag: cflags.FlagBroadcastMode,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.BroadcastMode
			},
			isTxOnly: true,
		},
		{
			flag: cflags.FlagSkipConfirmation,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.SkipConfirm
			},
			isTxOnly: true,
		},
		{
			flag: cflags.FlagSignMode,
			ctxFieldGetter: func(ctx client.Context) interface{} {
				return ctx.SignModeStr
			},
			isTxOnly: true,
		},
		// {
		// 	flag: cli.FlagFeeAccount,
		// 	ctxFieldGetter: func(ctx client.Context) interface{} {
		// 		return ctx.FeeGranter.String()
		// 	},
		// 	isTxOnly: true,
		// },
		{
			flag: cflags.FlagFrom,
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
	cmd.PersistentFlags().String(cflags.FlagHome, app.DefaultHome, "The application home directory")
	cmd.PersistentFlags().String(cflags.FlagChainID, "", "The network chain ID")
	cmd.Flags().Int64(cflags.FlagHeight, 0, "Use a specific height to query state at (this can error if the node is pruning state)")
	cflags.AddTxFlagsToCmd(cmd)

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
