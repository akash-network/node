package common

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func AddDepositFlags(flags *pflag.FlagSet, dflt sdk.Coin) {
	flags.String("deposit", dflt.String(), "Deposit amount")
}

func DepositFromFlags(flags *pflag.FlagSet) (sdk.Coin, error) {
	val, err := flags.GetString("deposit")
	if err != nil {
		return sdk.Coin{}, err
	}
	return sdk.ParseCoinNormalized(val)
}

func MarkReqDepositFlags(cmd *cobra.Command) {
	_ = cmd.MarkFlagRequired("deposit")
}
