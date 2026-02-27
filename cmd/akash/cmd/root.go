package cmd

import (
	"context"

	"github.com/CosmWasm/wasmd/x/wasm"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	"github.com/spf13/cobra"

	cmtcli "github.com/cometbft/cometbft/libs/cli"

	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/pruning"
	"github.com/cosmos/cosmos-sdk/client/snapshot"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	rosettaCmd "github.com/cosmos/rosetta/cmd"

	"pkg.akt.dev/go/cli"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/app"
	"pkg.akt.dev/node/v2/cmd/akash/cmd/testnetify"
)

// NewRootCmd creates a new root command for akash. It is called once in the
// main function.
func NewRootCmd() (*cobra.Command, sdkutil.EncodingConfig) {
	encodingConfig := sdkutil.MakeEncodingConfig()
	app.ModuleBasics().RegisterInterfaces(encodingConfig.InterfaceRegistry)

	rootCmd := &cobra.Command{
		Use:   "akash",
		Short: "Akash Blockchain Application",
		Long: `Akash CLI Utility.

Akash is a peer-to-peer marketplace for computing resources and
a deployment platform for heavily distributed applications.
Find out more at https://akash.network`,
		SilenceUsage:      true,
		PersistentPreRunE: cli.GetPersistentPreRunE(encodingConfig, []string{"AKASH"}, cli.DefaultHome, cli.WithPreRunAppConfig(InitAppConfig())),
	}

	initRootCmd(rootCmd, encodingConfig)

	return rootCmd, encodingConfig
}

// Execute executes the root command.
func Execute(rootCmd *cobra.Command, envPrefix string) error {
	// Create and set a client.Context on the command's Context. During the pre-run
	// of the root command, a default initialized client.Context is provided to
	// seed child command execution with values such as AccountRetriever, Keyring,
	// and a Tendermint RPC. This requires the use of a pointer reference when
	// getting and setting the client.Context. Ideally, we utilize
	// https://github.com/spf13/cobra/pull/1118.

	return cli.ExecuteWithCtx(context.Background(), rootCmd, envPrefix)
}

func initRootCmd(rootCmd *cobra.Command, encodingConfig sdkutil.EncodingConfig) {
	ac := appCreator{encodingConfig}

	home := app.DefaultHome

	debugCmd := debug.Cmd()
	debugCmd.AddCommand(ConvertBech32Cmd())

	rootCmd.AddCommand(
		sdkserver.StatusCommand(),
		AuthCmd(),
		cli.EventsCmd(),
		cli.QueryCmd(),
		cli.TxCmd(),
		cli.KeysCmds(),
		genesisCommand(encodingConfig),
		cmtcli.NewCompletionCmd(rootCmd, true),
		debugCmd,
		rosettaCmd.RosettaCommand(encodingConfig.InterfaceRegistry, encodingConfig.Codec),
		pruning.Cmd(ac.newApp, home),
		snapshot.Cmd(ac.newApp),
		testnetCmd(app.ModuleBasics(), banktypes.GenesisBalancesIterator{}),
		PrepareGenesisCmd(app.DefaultHome, app.ModuleBasics()),
		testnetify.GetCmd(ac.newTestnetApp),
	)

	cli.ServerCmds(rootCmd, home, ac.newApp, ac.appExport, addModuleInitFlags)

	rootCmd.SetOut(rootCmd.OutOrStdout())
	rootCmd.SetErr(rootCmd.ErrOrStderr())
}

func addModuleInitFlags(startCmd *cobra.Command) {
	crisis.AddModuleInitFlags(startCmd) //nolint: staticcheck
	wasm.AddModuleInitFlags(startCmd)
}

// genesisCommand builds genesis-related `simd genesis` command. Users may provide application specific commands as a parameter
func genesisCommand(encodingConfig sdkutil.EncodingConfig, cmds ...*cobra.Command) *cobra.Command {
	home := app.DefaultHome

	cmd := cli.GetGenesisCmd(app.ModuleBasics(), encodingConfig.TxConfig, app.DefaultHome, encodingConfig.SigningOptions.ValidatorAddressCodec)

	for _, subCmd := range cmds {
		cmd.AddCommand(subCmd)
	}

	cmd.AddCommand(AddGenesisAccountCmd(home))

	return cmd
}
