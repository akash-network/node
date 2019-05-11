package main

import (
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gosuri/uitable"
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

	printerDat := session.NewPrinterDataKV().
		AddResultKV("node_version", version).
		AddResultKV("latest_block_height", strconv.FormatInt(syncHeight, 10))
	if syncHeight > 0 {
		printerDat.
			AddResultKV("latest_block_hash", syncHash).
			AddResultKV("lastest_block_time", syncBlockTime.Format(time.RFC3339))
	}
	printerDat.Raw = result

	return s.Mode().
		When(session.ModeTypeInteractive, func() error {
			t := uitable.New().
				AddRow("Node Version: ", version).
				AddRow("Latest Block (Height): ", humanize.Comma(syncHeight))
			if syncHeight > 0 {
				t.AddRow("Latest Block Hash: ", syncHash).
					AddRow("Last Block Created: ", humanize.Time(syncBlockTime))
			}
			return session.NewIPrinter(nil).AddText("").Add(t).Flush()
		}).
		When(session.ModeTypeText, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(session.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}
