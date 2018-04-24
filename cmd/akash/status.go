package main

import (
	"fmt"
	"os"

	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/spf13/cobra"
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
	client := ctx.Client()

	result, err := client.Status()
	if err != nil {
		return err
	}

	fmt.Printf("Block: %v\nBlock Hash: %v\n", result.SyncInfo.LatestBlockHeight, result.SyncInfo.LatestBlockHash)

	if result.SyncInfo.LatestBlockHeight == 0 {
		os.Exit(1)
	}

	return nil
}
