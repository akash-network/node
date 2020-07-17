package types

import (
	cflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	flagAfter = "after"
)

func AddPaginationFlags(flags *pflag.FlagSet) {
	flags.Bool(flagAfter, false, "")
	flags.Int(cflags.FlagLimit, 10, "")

	_ = viper.BindPFlag(flagAfter, flags.Lookup(flagAfter))
	_ = viper.BindPFlag(cflags.FlagLimit, flags.Lookup(cflags.FlagLimit))
}
