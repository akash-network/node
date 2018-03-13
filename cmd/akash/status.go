package main

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/spf13/cobra"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func statusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "get remote node status",
		Args:  cobra.NoArgs,
		RunE:  context.WithContext(context.RequireNode(doStatusCommand)),
	}
	context.AddFlagNode(cmd, cmd.Flags())
	return cmd
}

func doStatusCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	client := tmclient.NewHTTP(ctx.Node(), "/websocket")

	result, err := client.Status()
	if err != nil {
		return err
	}
	fmt.Printf("Block: %v\nBlock Hash: %v\n", result.LatestBlockHeight, result.LatestBlockHash)

	return nil
}
