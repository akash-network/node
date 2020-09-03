package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cosmos/cosmos-sdk/client/flags"
)

func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Akash provider commands",
	}

	cmd.PersistentFlags().String(flags.FlagNode, "http://localhost:26657", "The node address")
	if err := viper.BindPFlag(flags.FlagNode, cmd.PersistentFlags().Lookup(flags.FlagNode)); err != nil {
		return nil
	}

	cmd.AddCommand(sendManifestCmd())
	cmd.AddCommand(statusCmd())
	cmd.AddCommand(leaseStatusCmd())
	cmd.AddCommand(serviceStatusCmd())
	cmd.AddCommand(serviceLogsCmd())
	cmd.AddCommand(runCmd())

	return cmd
}
