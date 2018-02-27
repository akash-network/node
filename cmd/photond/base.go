package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	envPrefix = "PHOTOND"
)

func initEnv(path string) error {
	if path != "" {
		viper.SetConfigName("config")
		viper.SetConfigType("toml")
		viper.AddConfigPath(path)
	}
	viper.SetEnvPrefix("PHOTOND")

	viper.BindEnv("p2p.seeds")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	return nil
}

func baseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "photond",
		Short: "Photon node",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			root, err := cmd.Flags().GetString(flagRootDir)
			if err != nil {
				return err
			}
			return initEnv(root)
		},
	}

	cmd.PersistentFlags().StringP(flagRootDir, "d", defaultRootDir(), "data directory")

	return cmd
}

func defaultRootDir() string {
	if val := os.Getenv(envPrefix + "_DATA"); val != "" {
		return val
	}
	return os.ExpandEnv("$HOME/.photond")
}
