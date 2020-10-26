package main

import (
	"net/http"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/lcd"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/version"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	authrest "github.com/cosmos/cosmos-sdk/x/auth/client/rest"
	bankcmd "github.com/cosmos/cosmos-sdk/x/bank/client/cli"
	"github.com/rakyll/statik/fs"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/go-amino"
	"github.com/tendermint/tendermint/libs/cli"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/cmd/common"

	csupply "github.com/ovrclk/cosmos-circulating-supply/x/supply/client/cli"

	// unnamed import of statik for swagger UI support
	_ "github.com/ovrclk/akash/cmd/statik"
)

func main() {

	common.InitSDKConfig()

	cdc := app.MakeCodec()

	root := &cobra.Command{
		Use:   "akashctl",
		Short: "Akash is a supercloud for serverless computing",
		Long:  "Akash Network CLI Utility.\n\nAkash is a peer-to-peer marketplace for computing resources and \na deployment platform for heavily distributed applications. \nFind out more at https://akash.network",
	}

	// Add --chain-id to persistent flags and mark it required
	root.PersistentFlags().String(flags.FlagChainID, "", "Chain ID of tendermint node")

	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
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

		return nil
	}

	root.AddCommand(
		rpc.StatusCommand(),
		client.ConfigCmd(common.DefaultCLIHome()),
		queryCmd(cdc),
		txCmd(cdc),
		lcd.ServeCommand(cdc, lcdRoutes),
		keys.Commands(),
		version.Cmd,
		flags.NewCompletionCmd(root, true),
	)

	addOtherCommands(root, cdc)

	executor := cli.PrepareMainCmd(root, "AKASHCTL", common.DefaultCLIHome())
	err := executor.Execute()
	if err != nil {
		panic(err)
	}
}

func queryCmd(cdc *amino.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "query",
		Aliases: []string{"q"},
		Short:   "Querying subcommands",
	}

	cmd.AddCommand(
		authcmd.GetAccountCmd(cdc),
		flags.LineBreak,
		rpc.ValidatorCommand(cdc),
		rpc.BlockCommand(),
		authcmd.QueryTxsByEventsCmd(cdc),
		authcmd.QueryTxCmd(cdc),
		flags.LineBreak,
	)

	app.ModuleBasics().AddQueryCommands(cmd, cdc)

	// Add circulating query in supply SDK queries
	supplyCmd, _, _ := cmd.Find([]string{"supply"})
	supplyCmd.AddCommand(csupply.GetCirculatingSupply(cdc))

	return cmd
}

func txCmd(cdc *amino.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx",
		Short: "Transactions subcommands",
	}

	cmd.AddCommand(
		bankcmd.SendTxCmd(cdc),
		flags.LineBreak,
		authcmd.GetSignCommand(cdc),
		authcmd.GetMultiSignCommand(cdc),
		flags.LineBreak,
		authcmd.GetBroadcastCommand(cdc),
		authcmd.GetEncodeCommand(cdc),
		flags.LineBreak,
	)

	// add modules' tx commands
	app.ModuleBasics().AddTxCommands(cmd, cdc)

	return cmd
}

func lcdRoutes(rs *lcd.RestServer) {
	client.RegisterRoutes(rs.CliCtx, rs.Mux)
	authrest.RegisterTxRoutes(rs.CliCtx, rs.Mux)
	app.ModuleBasics().RegisterRESTRoutes(rs.CliCtx, rs.Mux)
	registerSwaggerUI(rs)
}

func registerSwaggerUI(rs *lcd.RestServer) {
	statikFS, err := fs.New()
	if err != nil {
		panic(err)
	}
	staticServer := http.FileServer(statikFS)
	rs.Mux.PathPrefix("/").Handler(staticServer)
}
