package session

import (
	"os"
	"path"

	"github.com/cosmos/cosmos-sdk/crypto/keys"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagRootDir  = "data"
	flagNode     = "node"
	flagNonce    = "nonce"
	flagKey      = "key"
	flagKeyType  = "type"
	flagNoWait   = "no-wait"
	flagHost     = "host"
	flagPassword = "password"
	keyDir       = "keys"

	defaultKeyType  = keys.Secp256k1
	defaultPassword = "0123456789"
	defaultHost     = "localhost"
	defaultNode     = "http://api.akashtest.net:80"
)

func SetupBaseCommand(cmd *cobra.Command) {
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		root, _ := cmd.Flags().GetString(flagRootDir)
		return initCommandConfig(root)
	}
	cmd.PersistentFlags().StringP(flagRootDir, "d", defaultRootDir(), "data directory")
}

func initCommandConfig(root string) error {
	viper.SetEnvPrefix("AKASH")

	viper.BindEnv(flagNode)

	viper.BindEnv(flagPassword)
	viper.SetDefault(flagPassword, defaultPassword)

	viper.AutomaticEnv()
	viper.SetConfigFile(path.Join(root, "akash.toml"))

	if err := viper.ReadInConfig(); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func defaultRootDir() string {
	if val := os.Getenv("AKASH_DATA"); val != "" {
		return val
	}
	return os.ExpandEnv("$HOME/.akash")
}
