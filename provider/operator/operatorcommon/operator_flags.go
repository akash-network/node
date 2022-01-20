package operatorcommon

import (
	provider_flags "github.com/ovrclk/akash/provider/cmd/flags"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"time"
)

func AddOperatorFlags(cmd *cobra.Command, defaultListenAddress string) {
	cmd.Flags().String(provider_flags.FlagK8sManifestNS, "lease", "Cluster manifest namespace")
	if err := viper.BindPFlag(provider_flags.FlagK8sManifestNS, cmd.Flags().Lookup(provider_flags.FlagK8sManifestNS)); err != nil {
		panic(err)
	}

	cmd.Flags().String(provider_flags.FlagListenAddress, defaultListenAddress, "listen address for web server")
	if err := viper.BindPFlag(provider_flags.FlagListenAddress, cmd.Flags().Lookup(provider_flags.FlagListenAddress)); err != nil {
		panic(err)
	}

	cmd.Flags().Duration(provider_flags.FlagPruneInterval, 10*time.Minute, "data pruning interval")
	if err := viper.BindPFlag(provider_flags.FlagPruneInterval, cmd.Flags().Lookup(provider_flags.FlagPruneInterval)); err != nil {
		panic(err)
	}

	cmd.Flags().Duration(provider_flags.FlagWebRefreshInterval, 5*time.Second, "web data refresh interval")
	if err := viper.BindPFlag(provider_flags.FlagWebRefreshInterval, cmd.Flags().Lookup(provider_flags.FlagWebRefreshInterval)); err != nil {
		panic(err)
	}

	cmd.Flags().Duration(provider_flags.FlagRetryDelay, 3*time.Second, "retry delay")
	if err := viper.BindPFlag(provider_flags.FlagRetryDelay, cmd.Flags().Lookup(provider_flags.FlagRetryDelay)); err != nil {
		panic(err)
	}
}
