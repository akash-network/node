package context

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tendermint/go-crypto/keys"
)

func AddFlagNode(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(constants.FlagNode, "n", "http://localhost:46657", "node host")
	viper.BindPFlag(constants.FlagNode, flags.Lookup(constants.FlagNode))
}

func AddFlagKey(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(constants.FlagKey, "k", "", "key name (required)")
	cmd.MarkFlagRequired(constants.FlagKey)
}

func AddFlagNonce(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.Uint64(constants.FlagNonce, 0, "nonce (optional)")
}

func AddFlagKeyType(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(constants.FlagKeyType, "t", "ed25519", "Type of key (ed25519|secp256k1|ledger)")
}

func AddFlagWait(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.BoolP(constants.FlagWait, "w", false, "Wait for market confirmation")
}

func parseFlagKeyType(flags *pflag.FlagSet) (keys.CryptoAlgo, error) {
	ktype, err := flags.GetString(constants.FlagKeyType)
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
