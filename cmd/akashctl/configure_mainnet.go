// +build mainnet

package main

import (
	ecmd "github.com/ovrclk/akash/events/cmd"
	"github.com/spf13/cobra"
)

func addOtherCommands(root *cobra.Command) {
	root.AddCommand(
		ecmd.EventCmd(),
	)
}
