package main

import (
	"os"
	"path"

	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func initEnv(root string) error {
	viper.SetEnvPrefix("PHOTON")
	viper.AutomaticEnv()

	viper.SetConfigFile(path.Join(root, "photon.toml"))

	if err := viper.ReadInConfig(); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func saveConfig() error {
	return viper.WriteConfig()
}

func baseCommand() *cobra.Command {
	viper.SetEnvPrefix("PHOTON")

	cmd := &cobra.Command{
		Use:   "photon",
		Short: "Photon client",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			root, _ := cmd.Flags().GetString(constants.FlagRootDir)
			return initEnv(root)
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			return saveConfig()
		},
	}

	cmd.PersistentFlags().StringP(constants.FlagRootDir, "d", defaultRootDir(), "data directory")

	return cmd
}

func defaultRootDir() string {
	if val := os.Getenv("PHOTON_DATA"); val != "" {
		return val
	}
	return os.ExpandEnv("$HOME/.photon")
}
