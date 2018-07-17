package session

import (
	"os"
	"path"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagRootDir = "data"
	flagNode    = "node"
	flagNonce   = "nonce"
	flagKey     = "key"
	keyDir      = "keys"
	codec       = "english"
	flagKeyType = "type"
	keyType     = "ed25519"
	flagNoWait  = "no-wait"
	flagHost    = "host"
)

func SetupBaseCommand(cmd *cobra.Command) {
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		root, _ := cmd.Flags().GetString(flagRootDir)
		return initCommandConfig(root)
	}
	cmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		return saveCommandConfig()
	}
	cmd.PersistentFlags().StringP(flagRootDir, "d", defaultRootDir(), "data directory")
}

func initCommandConfig(root string) error {
	viper.SetEnvPrefix("AKASH")

	viper.BindEnv(flagNode)
	viper.SetDefault(flagNode, "testnet.akash.network")

	viper.BindEnv("password")
	viper.SetDefault("password", "0123456789")

	viper.BindEnv("host")

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
	if val := os.Getenv("AKASH_DATA"); val != "" {
		return val
	}
	return os.ExpandEnv("$HOME/.akash")
}
