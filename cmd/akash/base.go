package main

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/spf13/cobra"
)

func baseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "akash",
		Short: "Akash client",
	}
	context.SetupBaseCommand(cmd)
	return cmd
}
