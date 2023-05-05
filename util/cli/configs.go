package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	tmcfg "github.com/tendermint/tendermint/config"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/config"
)

const (
	FlagLogColor     = "log_color"
	FlagLogTimestamp = "log_timestamp"
)

var (
	ErrEmptyEnvPrefix = errors.New("envPrefixes parameter must contain at least one prefix")
)

// InterceptConfigsPreRunHandler performs a pre-run function for the root daemon
// application command. It will create a Viper literal and a default server
// Context. The server Tendermint configuration will either be read and parsed
// or created and saved to disk, where the server Context is updated to reflect
// the Tendermint configuration. It takes custom app config template and config
// settings to create a custom Tendermint configuration. If the custom template
// is empty, it uses default-template provided by the server. The Viper literal
// is used to read and parse the application configuration. Command handlers can
// fetch the server Context to get the Tendermint configuration or to get access
// to Viper.
func InterceptConfigsPreRunHandler(
	cmd *cobra.Command,
	envPrefixes []string,
	allowEmptyEnv bool,
	customAppConfigTemplate string,
	customAppConfig interface{},
) error {
	if len(envPrefixes) == 0 {
		return ErrEmptyEnvPrefix
	}

	serverCtx := server.NewDefaultContext()

	// Configure the viper instance
	if err := serverCtx.Viper.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	if err := serverCtx.Viper.BindPFlags(cmd.PersistentFlags()); err != nil {
		return err
	}

	serverCtx.Viper.SetEnvPrefix(envPrefixes[0])
	serverCtx.Viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	serverCtx.Viper.AllowEmptyEnv(allowEmptyEnv)
	serverCtx.Viper.AutomaticEnv()

	// intercept configuration files, using both Viper instances separately
	cfg, err := interceptConfigs(serverCtx.Viper, customAppConfigTemplate, customAppConfig)
	if err != nil {
		return err
	}
	serverCtx.Config = cfg

	logTimeFmt, err := parseTimestampFormat(serverCtx.Viper.GetString(FlagLogTimestamp))
	if err != nil {
		return err
	}

	logLvlStr := serverCtx.Viper.GetString(flags.FlagLogLevel)
	serverCtx.Viper.GetString(flags.FlagLogLevel)
	logLvl, err := zerolog.ParseLevel(logLvlStr)
	if err != nil {
		return fmt.Errorf("failed to parse log level (%s): %w", logLvlStr, err)
	}

	logWriter := io.Writer(os.Stdout)

	if strings.ToLower(serverCtx.Viper.GetString(flags.FlagLogFormat)) == tmcfg.LogFormatPlain {
		cl := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			NoColor:    !serverCtx.Viper.GetBool(FlagLogColor),
			TimeFormat: logTimeFmt,
		}

		if logTimeFmt == "" {
			cl.PartsExclude = []string{
				zerolog.TimestampFieldName,
			}
		}
		logWriter = cl
	}

	logger := zerolog.New(logWriter).Level(logLvl)

	if logTimeFmt != "" {
		logger = logger.With().Timestamp().Logger()
	}

	serverCtx.Logger = server.ZeroLogWrapper{Logger: logger}

	if err = bindFlags(cmd, serverCtx.Viper, envPrefixes); err != nil {
		return err
	}

	return server.SetCmdServerContext(cmd, serverCtx)
}

func parseTimestampFormat(val string) (string, error) {
	switch val {
	case "":
		return "", nil
	case "rfc3339":
		return time.RFC3339, nil
	case "rfc3339nano":
		return time.RFC3339Nano, nil
	case "kitchen":
		return time.Kitchen, nil
	}

	return "", fmt.Errorf("invalid timestamp format (%s)", val) // nolint goerr113
}

func bindFlags(cmd *cobra.Command, v *viper.Viper, envPrefixes []string) error {
	var err error

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores, e.g. --favorite-color to STING_FAVORITE_COLOR
		envBody := strings.ReplaceAll(f.Name, "-", "_")
		envBody = strings.ToUpper(strings.ReplaceAll(envBody, ".", "_"))

		for _, prefix := range envPrefixes {
			env := fmt.Sprintf("%s_%s", prefix, envBody)
			if err = v.BindEnv(f.Name, env); err != nil {
				return
			}
		}

		err = v.BindPFlag(f.Name, f)
		if err != nil {
			return
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			if err = cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				return
			}
		}
	})

	if err != nil {
		return err
	}

	return nil
}

// interceptConfigs parses and updates a Tendermint configuration file or
// creates a new one and saves it. It also parses and saves the application
// configuration file. The Tendermint configuration file is parsed given a root
// Viper object, whereas the application is parsed with the private package-aware
// viperCfg object.
func interceptConfigs(rootViper *viper.Viper, customAppTemplate string, customConfig interface{}) (*tmcfg.Config, error) {
	rootDir := rootViper.GetString(flags.FlagHome)
	configPath := filepath.Join(rootDir, "config")
	tmCfgFile := filepath.Join(configPath, "config.toml")

	conf := tmcfg.DefaultConfig()

	switch _, err := os.Stat(tmCfgFile); {
	case os.IsNotExist(err):
		tmcfg.EnsureRoot(rootDir)

		if err = conf.ValidateBasic(); err != nil {
			return nil, fmt.Errorf("error in config file: %v", err) // nolint: goerr113
		}

		conf.RPC.PprofListenAddress = "localhost:6060"
		conf.P2P.RecvRate = 5120000
		conf.P2P.SendRate = 5120000
		conf.Consensus.TimeoutCommit = 5 * time.Second
		tmcfg.WriteConfigFile(tmCfgFile, conf)

	case err != nil:
		return nil, err

	default:
		rootViper.SetConfigType("toml")
		rootViper.SetConfigName("config")
		rootViper.AddConfigPath(configPath)

		if err := rootViper.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read in %s: %w", tmCfgFile, err) // nolint: goerr113
		}
	}

	// Read into the configuration whatever data the viper instance has for it.
	// This may come from the configuration file above but also any of the other
	// sources viper uses.
	if err := rootViper.Unmarshal(conf); err != nil {
		return nil, err
	}

	conf.SetRoot(rootDir)

	appCfgFilePath := filepath.Join(configPath, "app.toml")
	if _, err := os.Stat(appCfgFilePath); os.IsNotExist(err) {
		if customAppTemplate != "" {
			config.SetConfigTemplate(customAppTemplate)

			if err = rootViper.Unmarshal(&customConfig); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", appCfgFilePath, err) // nolint: goerr113
			}

			config.WriteConfigFile(appCfgFilePath, customConfig)
		} else {
			appConf, err := config.ParseConfig(rootViper)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", appCfgFilePath, err) // nolint: goerr113
			}

			config.WriteConfigFile(appCfgFilePath, appConf)
		}
	}

	rootViper.SetConfigType("toml")
	rootViper.SetConfigName("app")
	rootViper.AddConfigPath(configPath)

	if err := rootViper.MergeInConfig(); err != nil {
		return nil, fmt.Errorf("failed to merge configuration: %w", err) // nolint: goerr113
	}

	return conf, nil
}
