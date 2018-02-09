package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func initEnv() {
	viper.SetEnvPrefix("PHOTOND")
	viper.AutomaticEnv()
}

func baseCommand() *cobra.Command {
	cobra.OnInitialize(initEnv)

	cmd := &cobra.Command{
		Use:   "photond",
		Short: "Photon node",
	}

	cmd.PersistentFlags().StringP(flagRootDir, "d", defaultRootDir(), "data directory")

	return cmd
}

func defaultRootDir() string {
	if val := os.Getenv("PHOTOND_DATA"); val != "" {
		return val
	}
	return os.ExpandEnv("$HOME/.photond")
}
