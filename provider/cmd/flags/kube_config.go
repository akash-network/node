package flags

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func AddKubeConfigPathFlag(cmd *cobra.Command) error {
	cmd.Flags().String(FlagKubeConfig, "", "kubernetes configuration file path")
	return viper.BindPFlag(FlagKubeConfig, cmd.Flags().Lookup(FlagKubeConfig))
}
