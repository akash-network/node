package context

import (
	"os"
	"path"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func SetupBaseCommand(cmd *cobra.Command) {
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		root, _ := cmd.Flags().GetString(constants.FlagRootDir)
		return initCommandConfig(root)
	}
	cmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		return saveCommandConfig()
	}
	cmd.PersistentFlags().StringP(constants.FlagRootDir, "d", defaultRootDir(), "data directory")
}

func initCommandConfig(root string) error {
	viper.SetEnvPrefix("PHOTON")
	viper.AutomaticEnv()
	viper.SetConfigFile(path.Join(root, "akash.toml"))

	if err := viper.ReadInConfig(); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveCommandConfig() error {
	return viper.WriteConfig()
}

func defaultRootDir() string {
	if val := os.Getenv("PHOTON_DATA"); val != "" {
		return val
	}
	return os.ExpandEnv("$HOME/.akash")
}
