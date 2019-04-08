package main

import (
	"fmt"
	"os"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/spf13/cobra"
)

func statusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display node status",
		Args:  cobra.NoArgs,
		RunE:  session.WithSession(session.RequireNode(doStatusCommand)),
	}
	session.AddFlagNode(cmd, cmd.Flags())
	return cmd
}

func doStatusCommand(session session.Session, cmd *cobra.Command, args []string) error {
	client := session.Client()

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
