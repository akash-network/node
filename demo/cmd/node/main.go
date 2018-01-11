package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/tendermint/tmlibs/cli"

	client "github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/server/commands"
	counter "github.com/ovrclk/photon/demo/plugins/order"
)

// RootCmd is the entry point for this binary
var RootCmd = &cobra.Command{
	Use:   "counter",
	Short: "demo application for cosmos sdk",
}

func main() {

	// TODO: register the counter here
	commands.Handler = counter.NewHandler("mycoin")

	RootCmd.AddCommand(
		commands.InitCmd,
		commands.StartCmd,
		commands.UnsafeResetAllCmd,
		client.VersionCmd,
	)
	commands.SetUpRoot(RootCmd)

	// "CT" is an environment prefix. should be unique per abci app
	cmd := cli.PrepareMainCmd(RootCmd, "CT", os.ExpandEnv("$HOME/.counter"))
	cmd.Execute()
}
