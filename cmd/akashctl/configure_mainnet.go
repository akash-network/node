// +build mainnet

package main

import (
	"github.com/cosmos/cosmos-sdk/codec"
	ecmd "github.com/ovrclk/akash/events/cmd"
	"github.com/spf13/cobra"
)

func addOtherCommands(root *cobra.Command, cdc *codec.Codec) {
	root.AddCommand(
		ecmd.EventCmd(cdc),
	)
}
