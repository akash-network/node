package main

import (
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/ovrclk/akash/cmd/akash/session"
	. "github.com/ovrclk/akash/util"
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

func doStatusCommand(s session.Session, cmd *cobra.Command, args []string) error {
	client := s.Client()
	result, err := client.Status()
	if err != nil {
		return err
	}
	version := result.NodeInfo.Version
	syncHeight := result.SyncInfo.LatestBlockHeight
	syncHash := X(result.SyncInfo.LatestBlockHash)
	syncBlockTime := result.SyncInfo.LatestBlockTime

	printer := s.Mode().Printer()
	data := printer.NewSection("Status").NewData().AsPane().
		Add("Node Version", version).
		Add("Latest Block Height", strconv.FormatInt(syncHeight, 10))
	if syncHeight > 0 {
		data.
			Add("Latest Block Hash", syncHash).
			Add("Last Block Created", humanize.Time(syncBlockTime))
	}
	data.WithTag("raw", result)
	return printer.Flush()
}
