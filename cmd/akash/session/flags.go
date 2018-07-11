package session

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/crypto/keys"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func AddFlagNode(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(flagNode, "n", defaultNode, "node host")
	viper.BindPFlag(flagNode, flags.Lookup(flagNode))
}

func AddFlagKey(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(flagKey, "k", "", "key name (required)")
	cmd.MarkFlagRequired(flagKey)
}

func AddFlagKeyOptional(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(flagKey, "k", "", "key name")
}

func AddFlagNonce(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.Uint64(flagNonce, 0, "nonce (optional)")
}

func AddFlagKeyType(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(flagKeyType, "t", string(defaultKeyType), "Type of key (secp256k1|ed25519|ledger)")
}

func AddFlagWait(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.Bool(flagNoWait, false, "Do not wait for lease creation")
}

func AddFlagHost(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.String(flagHost, defaultHost, "cluster host")
	viper.BindPFlag(flagHost, flags.Lookup(flagHost))
}

func parseFlagKeyType(flags *pflag.FlagSet) (keys.SigningAlgo, error) {
	ktype, err := flags.GetString(flagKeyType)
	if err != nil {
		return "", err
	}

	switch keys.SigningAlgo(ktype) {
	case keys.Ed25519:
		return keys.Ed25519, nil
	case keys.Secp256k1:
		return keys.Secp256k1, nil
	default:
		return "", fmt.Errorf("unknown key type: %v", ktype)
	}
}
