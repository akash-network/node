package main

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/spf13/cobra"
)

func baseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "akash",
		Short:        "Akash client",
		SilenceUsage: true,
	}
	session.SetupBaseCommand(cmd)
	return cmd
}
