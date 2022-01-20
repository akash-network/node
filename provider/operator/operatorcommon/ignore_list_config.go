package operatorcommon

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"time"
)

const (
	FlagIgnoreListEntryLimit = "ignore-list-entry-limit"
	FlagIgnoreListAgeLimit   = "ignore-list-age-limit"
	FlagEventFailureLimit    = "event-failure-limit"
)

type IgnoreListConfig struct {
	// This is a config object, so it isn't exported as an interface
	FailureLimit uint
	EntryLimit   uint
	AgeLimit     time.Duration
}

func IgnoreListConfigFromViper() IgnoreListConfig {
	return IgnoreListConfig{
		FailureLimit: viper.GetUint(FlagEventFailureLimit),
		EntryLimit:   viper.GetUint(FlagIgnoreListEntryLimit),
		AgeLimit:     viper.GetDuration(FlagIgnoreListAgeLimit),
	}
}

func AddIgnoreListFlags(cmd *cobra.Command) {
	cmd.Flags().Uint(FlagIgnoreListEntryLimit, 131072, "ignore list size limit")
	if err := viper.BindPFlag(FlagIgnoreListEntryLimit, cmd.Flags().Lookup(FlagIgnoreListEntryLimit)); err != nil {
		panic(err)
	}

	cmd.Flags().Duration(FlagIgnoreListAgeLimit, time.Hour*726, "ignore list entry age limit")
	if err := viper.BindPFlag(FlagIgnoreListAgeLimit, cmd.Flags().Lookup(FlagIgnoreListAgeLimit)); err != nil {
		panic(err)
	}

	cmd.Flags().Uint(FlagEventFailureLimit, 3, "event failure limit before it is ignored")
	if err := viper.BindPFlag(FlagEventFailureLimit, cmd.Flags().Lookup(FlagEventFailureLimit)); err != nil {
		panic(err)
	}
}
