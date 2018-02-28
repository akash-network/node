package context

import (
	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func AddFlagNode(cmd *cobra.Command, flags *pflag.FlagSet) {
	flags.StringP(constants.FlagNode, "n", constants.DefaultNode, "node host")
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
