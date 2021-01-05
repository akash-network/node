package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/tendermint/tendermint/libs/log"
)

var (
	// logger is the logger for the application
	logger = log.NewTMLogger(log.NewSyncWriter(os.Stdout))
)

// RootCmd represents root command of deploy tool
func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "deploy",
		Short:        "Akash deploy tool commands",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().String(flags.FlagNode, "http://localhost:26657", "The node address")
	if err := viper.BindPFlag(flags.FlagNode, cmd.PersistentFlags().Lookup(flags.FlagNode)); err != nil {
		return nil
	}

	cmd.AddCommand(createCmd())

	return cmd
}
