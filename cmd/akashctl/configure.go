// +build !mainnet

package main

import (
	ecmd "github.com/ovrclk/akash/events/cmd"
	pcmd "github.com/ovrclk/akash/provider/cmd"
	"github.com/spf13/cobra"
)

func addOtherCommands(root *cobra.Command) {
	root.AddCommand(
		pcmd.RootCmd(),
		ecmd.EventCmd(),
	)
}
