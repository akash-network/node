package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cosmos/cosmos-sdk/client/flags"
)

func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "provider",
		Short:        "Akash provider commands",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().String(flags.FlagNode, "http://localhost:26657", "The node address")
	if err := viper.BindPFlag(flags.FlagNode, cmd.PersistentFlags().Lookup(flags.FlagNode)); err != nil {
		return nil
	}

	cmd.AddCommand(SendManifestCmd())
	cmd.AddCommand(statusCmd())
	cmd.AddCommand(leaseStatusCmd())
	cmd.AddCommand(leaseEventsCmd())
	cmd.AddCommand(leaseLogsCmd())
	cmd.AddCommand(serviceStatusCmd())
	cmd.AddCommand(RunCmd())
	cmd.AddCommand(LeaseShellCmd())
	cmd.AddCommand(HostnameOperatorCmd())
	cmd.AddCommand(MigrateHostnamesCmd())
	cmd.AddCommand(AuthServerCmd())
	cmd.AddCommand(AuthenticateCmd())
	cmd.AddCommand(clusterNSCmd())
	cmd.AddCommand(migrate())
	cmd.AddCommand(RunResourceServerCmd())

	return cmd
}
