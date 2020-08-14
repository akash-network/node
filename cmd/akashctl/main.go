package main

import (
	"context"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/version"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankcmd "github.com/cosmos/cosmos-sdk/x/bank/client/cli"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/cli"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/cmd/common"

	// unnamed import of statik for swagger UI support
	_ "github.com/ovrclk/akash/cmd/statik"
)

func main() {

	common.InitSDKConfig()

	encodingConfig := app.MakeEncodingConfig()

	initClientCtx := client.Context{}.
		WithJSONMarshaler(encodingConfig.Marshaler).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithInput(os.Stdin).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode(flags.BroadcastBlock).
		WithHomeDir(common.DefaultCLIHome())

	root := &cobra.Command{
		Use:   "akashctl",
		Short: "Akash is a supercloud for serverless computing",
		Long:  "Akash Network CLI Utility.\n\nAkash is a peer-to-peer marketplace for computing resources and \na deployment platform for heavily distributed applications. \nFind out more at https://akash.network",
	}

	// Add --chain-id to persistent flags and mark it required
	root.PersistentFlags().String(flags.FlagChainID, "", "Chain ID of tendermint node")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		// viper bindings below should be applied to root command rather then
		// to an argument from this function. otherwise viper won't able to find them
		if err := viper.BindPFlag(flags.FlagChainID, root.PersistentFlags().Lookup(flags.FlagChainID)); err != nil {
			return err
		}

		if err := viper.BindPFlag(cli.EncodingFlag, root.PersistentFlags().Lookup(cli.EncodingFlag)); err != nil {
			return err
		}

		if err := viper.BindPFlag(cli.OutputFlag, root.PersistentFlags().Lookup(cli.OutputFlag)); err != nil {
			return err
		}

		if err := client.SetCmdClientContextHandler(initClientCtx, cmd); err != nil {
			return err
		}

		return nil
	}

	root.AddCommand(
		rpc.StatusCommand(),
		queryCmd(),
		txCmd(),
		// lcd.ServeCommand(cdc, lcdRoutes),
		keys.Commands(common.DefaultCLIHome()),
		version.NewVersionCommand(),
		cli.NewCompletionCmd(root, true),
	)

	addOtherCommands(root)

	ctx := context.Background()
	ctx = context.WithValue(ctx, client.ClientContextKey, &client.Context{})

	executor := cli.PrepareMainCmd(root, "AKASHCTL", common.DefaultCLIHome())
	err := executor.ExecuteContext(ctx)
	if err != nil {
		panic(err)
	}
}

func queryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "query",
		Aliases: []string{"q"},
		Short:   "Querying subcommands",
	}

	cmd.AddCommand(
		authcmd.GetAccountCmd(),
		flags.LineBreak,
		rpc.ValidatorCommand(),
		rpc.BlockCommand(),
		authcmd.QueryTxsByEventsCmd(),
		authcmd.QueryTxCmd(),
		flags.LineBreak,
	)

	app.ModuleBasics().AddQueryCommands(cmd)
	return cmd
}

func txCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx",
		Short: "Transactions subcommands",
	}

	cmd.AddCommand(
		bankcmd.NewSendTxCmd(),
		flags.LineBreak,
		authcmd.GetSignCommand(),
		authcmd.GetSignBatchCommand(),
		authcmd.GetMultiSignCommand(),
		authcmd.GetValidateSignaturesCommand(),
		flags.LineBreak,
		authcmd.GetBroadcastCommand(),
		authcmd.GetEncodeCommand(),
		authcmd.GetDecodeCommand(),
		flags.LineBreak,
	)

	// add modules' tx commands
	app.ModuleBasics().AddTxCommands(cmd)

	return cmd
}

// func lcdRoutes(rs *lcd.RestServer) {
// 	client.RegisterRoutes(rs.CliCtx, rs.Mux)
// 	authrest.RegisterTxRoutes(rs.CliCtx, rs.Mux)
// 	app.ModuleBasics().RegisterRESTRoutes(rs.CliCtx, rs.Mux)
// 	registerSwaggerUI(rs)
// }

// func registerSwaggerUI(rs *lcd.RestServer) {
// 	statikFS, err := fs.New()
// 	if err != nil {
// 		panic(err)
// 	}
// 	staticServer := http.FileServer(statikFS)
// 	rs.Mux.PathPrefix("/").Handler(staticServer)
// }
