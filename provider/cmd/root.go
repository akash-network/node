package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
)

func RootCmd(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Akash provider commands",
	}

	cmd.AddCommand(flags.PostCommands(
		runCmd(cdc),
	)...)

	return cmd
}
