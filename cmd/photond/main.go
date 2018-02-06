package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tendermint/tmlibs/cli"

	sdk "github.com/cosmos/cosmos-sdk"
	client "github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/modules/auth"
	"github.com/cosmos/cosmos-sdk/modules/base"
	"github.com/cosmos/cosmos-sdk/modules/coin"
	"github.com/cosmos/cosmos-sdk/modules/nonce"
	"github.com/cosmos/cosmos-sdk/server/commands"
	"github.com/cosmos/cosmos-sdk/stack"

	"github.com/ovrclk/photon/plugins/accounts"
)

// RootCmd is the entry point for this binary
var RootCmd = &cobra.Command{
	Use:   "photon",
	Short: "Blockchained infrustructure",
}

// BuildApp constructs the stack we want to use for this app
func BuildApp(feeDenom string) sdk.Handler {
	return stack.New(
		base.Logger{},
		stack.Recovery{},
		auth.Signatures{},
		base.Chain{},
		stack.Checkpoint{OnCheck: true},
		nonce.ReplayCheck{},
	).
		Dispatch(
			coin.NewHandler(),
			stack.WrapHandler(accounts.NewHandler()),
		)
}

func main() {
	// todo: create an issue in cosmos-sdk about custom coin names not working
	commands.Handler = BuildApp("photon")

	RootCmd.AddCommand(
		commands.InitCmd,
		commands.StartCmd,
		commands.UnsafeResetAllCmd,
		client.VersionCmd,
	)
	commands.SetUpRoot(RootCmd)

	cmd := cli.PrepareMainCmd(RootCmd, "BC", os.ExpandEnv("./data/node"))
	cmd.Execute()
}
