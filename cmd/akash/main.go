package main

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/lcd"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/version"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	authrest "github.com/cosmos/cosmos-sdk/x/auth/client/rest"
	bankcmd "github.com/cosmos/cosmos-sdk/x/bank/client/cli"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/spf13/cobra"
	"github.com/tendermint/go-amino"
	"github.com/tendermint/tendermint/libs/cli"
)

func main() {

	common.InitSDKConfig()

	cdc := app.MakeCodec()

	root := &cobra.Command{
		Use:   "akash",
		Short: "Akash is a supercloud for serverless computing",
		Long:  "Akash Network CLI Utility.\n\nAkash is a peer-to-peer marketplace for computing resources and \na deployment platform for heavily distributed applications. \nFind out more at https://akash.network",
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

	executor := cli.PrepareMainCmd(root, "AKASH", common.DefaultCLIHome())
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

	app.ModuleBasics.AddQueryCommands(cmd, cdc)
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
	app.ModuleBasics.AddTxCommands(cmd, cdc)

	return cmd
}

func lcdRoutes(rs *lcd.RestServer) {
	client.RegisterRoutes(rs.CliCtx, rs.Mux)
	authrest.RegisterTxRoutes(rs.CliCtx, rs.Mux)
	app.ModuleBasics.RegisterRESTRoutes(rs.CliCtx, rs.Mux)
}
