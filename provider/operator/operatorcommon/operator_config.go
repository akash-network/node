package operatorcommon

import (
	provider_flags "github.com/ovrclk/akash/provider/cmd/flags"
	"github.com/spf13/viper"
	"time"
)

type OperatorConfig struct {
	PruneInterval      time.Duration
	WebRefreshInterval time.Duration
	RetryDelay         time.Duration
	ProviderAddress    string
}

func GetOperatorConfigFromViper() OperatorConfig {
	return OperatorConfig{
		PruneInterval:      viper.GetDuration(provider_flags.FlagPruneInterval),
		WebRefreshInterval: viper.GetDuration(provider_flags.FlagWebRefreshInterval),
		RetryDelay:         viper.GetDuration(provider_flags.FlagRetryDelay),
		ProviderAddress:    viper.GetString(flagProviderAddress),
	}
}
