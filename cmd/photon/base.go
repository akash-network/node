package main

import (
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/spf13/cobra"
)

func baseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "photon",
		Short: "Photon client",
	}
	context.SetupBaseCommand(cmd)
	return cmd
}
