// +build !mainnet

package cmd

import (
	pcmd "github.com/ovrclk/akash/provider/cmd"
	"github.com/spf13/cobra"
)

func addOtherCommands(root *cobra.Command) {
	root.AddCommand(
		pcmd.RootCmd(),
	)
}
