package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/tendermint/tmlibs/cli"

	"github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/client/commands/auto"
	"github.com/cosmos/cosmos-sdk/client/commands/commits"
	"github.com/cosmos/cosmos-sdk/client/commands/keys"
	"github.com/cosmos/cosmos-sdk/client/commands/proxy"
	"github.com/cosmos/cosmos-sdk/client/commands/query"
	rpccmd "github.com/cosmos/cosmos-sdk/client/commands/rpc"
	"github.com/cosmos/cosmos-sdk/client/commands/search"
	txcmd "github.com/cosmos/cosmos-sdk/client/commands/txs"
	authcmd "github.com/cosmos/cosmos-sdk/modules/auth/commands"
	basecmd "github.com/cosmos/cosmos-sdk/modules/base/commands"
	coincmd "github.com/cosmos/cosmos-sdk/modules/coin/commands"
	noncecmd "github.com/cosmos/cosmos-sdk/modules/nonce/commands"

	accountscmd "github.com/ovrclk/photon/demo/plugins/accounts/commands"
)

// BaseCli - main basecoin client command
var BaseCli = &cobra.Command{
	Use:   "client",
	Short: "Light client for Tendermint",
}

func main() {
	commands.AddBasicFlags(BaseCli)

	// Prepare queries
	query.RootCmd.AddCommand(
		// These are default parsers, but optional in your app (you can remove key)
		query.TxQueryCmd,
		query.KeyQueryCmd,
		coincmd.AccountQueryCmd,
		noncecmd.NonceQueryCmd,
		accountscmd.AccountsQueryCmd,
	)

	// these are queries to search for a tx
	search.RootCmd.AddCommand(
		coincmd.SentSearchCmd,
	)

	// set up the middleware
	txcmd.Middleware = txcmd.Wrappers{
		noncecmd.NonceWrapper{},
		basecmd.ChainWrapper{},
		authcmd.SigWrapper{},
	}
	txcmd.Middleware.Register(txcmd.RootCmd.PersistentFlags())

	// you will always want this for the base send command
	txcmd.RootCmd.AddCommand(
		// This is the default transaction, optional in your app
		coincmd.SendTxCmd,
		accountscmd.SetTxCmd,
		accountscmd.RemoveTxCmd,
		// accountscmd.CreateTxCmd,
	)

	// Set up the various commands to use
	BaseCli.AddCommand(
		commands.InitCmd,
		commands.ResetCmd,
		keys.RootCmd,
		commits.RootCmd,
		rpccmd.RootCmd,
		query.RootCmd,
		search.RootCmd,
		txcmd.RootCmd,
		proxy.RootCmd,
		commands.VersionCmd,
		auto.AutoCompleteCmd,
	)

	cmd := cli.PrepareMainCmd(BaseCli, "BC", os.ExpandEnv("$HOME/.democlient"))
	cmd.Execute()
}
