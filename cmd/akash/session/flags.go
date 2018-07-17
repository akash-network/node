package session

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tendermint/go-crypto/keys"
)

func AddFlagNode(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(flagNode, "n", "http://localhost:46657", "node host")
	viper.BindPFlag(flagNode, flags.Lookup(flagNode))
}

func AddFlagKey(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(flagKey, "k", "", "key name (required)")
	cmd.MarkFlagRequired(flagKey)
}

func AddFlagNonce(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.Uint64(flagNonce, 0, "nonce (optional)")
}

func AddFlagKeyType(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(flagKeyType, "t", keyType, "Type of key (ed25519|secp256k1|ledger)")
}

func AddFlagWait(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.Bool(flagNoWait, false, "Do not wait for lease creation")
}

func parseFlagKeyType(flags *pflag.FlagSet) (keys.CryptoAlgo, error) {
	ktype, err := flags.GetString(flagKeyType)
	if err != nil {
		return "", err
	}

	switch keys.CryptoAlgo(ktype) {
	case keys.AlgoEd25519:
		return keys.AlgoEd25519, nil
	case keys.AlgoSecp256k1:
		return keys.AlgoSecp256k1, nil
	default:
		return "", fmt.Errorf("unknown key type: %v", ktype)
	}
}
