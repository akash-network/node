package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
)

func RootCmd(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Akash provider commands",
	}

	cmd.PersistentFlags().String(flags.FlagNode, "http://localhost:26657", "The node address")
	_ = viper.BindPFlag(flags.FlagNode, cmd.PersistentFlags().Lookup(flags.FlagNode))

	cmd.AddCommand(sendManifestCmd(cdc))
	cmd.AddCommand(statusCmd(cdc))
	cmd.AddCommand(leaseStatusCmd(cdc))
	cmd.AddCommand(serviceStatusCmd(cdc))
	cmd.AddCommand(serviceLogsCmd(cdc))
	cmd.AddCommand(flags.PostCommands(
		runCmd(cdc),
	)...)

	return cmd
}
