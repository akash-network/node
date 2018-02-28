package main

import (
	"fmt"

	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/spf13/cobra"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func pingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "ping remote node",
		Args:  cobra.NoArgs,
		RunE:  context.WithContext(context.RequireNode(doPingCommand)),
	}
	context.AddFlagNode(cmd, cmd.Flags())
	return cmd
}

func doPingCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	client := tmclient.NewHTTP(ctx.Node(), "/websocket")

	result, err := client.Status()
	if err != nil {
		return err
	}
	fmt.Printf("%v:%v\n", result.LatestBlockHeight, result.LatestBlockHash)

	return nil
}
