package main

import (
	"net/http"
	"os"
	"path"

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

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/cmd/common"
	pcmd "github.com/ovrclk/akash/provider/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/go-amino"
	"github.com/tendermint/tendermint/libs/cli"

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
		return initConfig(root)
	}

	root.AddCommand(
		rpc.StatusCommand(),
		client.ConfigCmd(common.DefaultCLIHome()),
		queryCmd(cdc),
		txCmd(cdc),
		lcd.ServeCommand(cdc, lcdRoutes),
		keys.Commands(),
		pcmd.RootCmd(cdc),
		version.Cmd,
		flags.NewCompletionCmd(root, true),
	)

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

func initConfig(cmd *cobra.Command) error {
	home, err := cmd.PersistentFlags().GetString(cli.HomeFlag)
	if err != nil {
		return err
	}

	cfgFile := path.Join(home, "config", "config.toml")
	if _, err := os.Stat(cfgFile); err == nil {
		viper.SetConfigFile(cfgFile)

		if err := viper.ReadInConfig(); err != nil {
			return err
		}
	}
	if err := viper.BindPFlag(flags.FlagChainID, cmd.PersistentFlags().Lookup(flags.FlagChainID)); err != nil {
		return err
	}
	if err := viper.BindPFlag(cli.EncodingFlag, cmd.PersistentFlags().Lookup(cli.EncodingFlag)); err != nil {
		return err
	}
	return viper.BindPFlag(cli.OutputFlag, cmd.PersistentFlags().Lookup(cli.OutputFlag))
}
