package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cosmossdk.io/log"
	perrors "github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	cflags "pkg.akt.dev/go/cli/flags"

	tmcfg "github.com/cometbft/cometbft/config"
	tmlog "github.com/cometbft/cometbft/libs/log"

	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/config"
)

var (
	ErrEmptyEnvPrefix = errors.New("envPrefixes parameter must contain at least one prefix")
)

type zeroLogWrapper struct {
	*zerolog.Logger
}

// Info takes a message and a set of key/value pairs and logs with level INFO.
// The key of the tuple must be a string.
func (l zeroLogWrapper) Info(msg string, keyVals ...interface{}) {
	l.Logger.Info().Fields(keyVals).Msg(msg)
}

// Warn takes a message and a set of key/value pairs and logs with level INFO.
// The key of the tuple must be a string.
func (l zeroLogWrapper) Warn(msg string, keyVals ...interface{}) {
	l.Logger.Warn().Fields(keyVals).Msg(msg)
}

// Error takes a message and a set of key/value pairs and logs with level DEBUG.
// The key of the tuple must be a string.
func (l zeroLogWrapper) Error(msg string, keyVals ...interface{}) {
	l.Logger.Error().Fields(keyVals).Msg(msg)
}

// Debug takes a message and a set of key/value pairs and logs with level ERR.
// The key of the tuple must be a string.
func (l zeroLogWrapper) Debug(msg string, keyVals ...interface{}) {
	l.Logger.Debug().Fields(keyVals).Msg(msg)
}

// With returns a new wrapped logger with additional context provided by a set.
func (l zeroLogWrapper) With(keyVals ...interface{}) log.Logger {
	logger := l.Logger.With().Fields(keyVals).Logger()
	return zeroLogWrapper{&logger}
}

// Impl returns the underlying zerolog logger.
// It can be used to used zerolog structured API directly instead of the wrapper.
func (l zeroLogWrapper) Impl() interface{} {
	return l.Logger
}

// NewLogger returns a new logger that writes to the given destination.
//
// Typical usage from a main function is:
//
//	logger := log.NewLogger(os.Stderr)
//
// Stderr is the typical destination for logs,
// so that any output from your application can still be piped to other processes.
func NewLogger(dst io.Writer, options ...log.Option) log.Logger {
	logCfg := log.Config{
		Level:      zerolog.NoLevel,
		Filter:     nil,
		OutputJSON: false,
		Color:      true,
		StackTrace: false,
		TimeFormat: time.Kitchen,
		Hooks:      nil,
	}

	for _, opt := range options {
		opt(&logCfg)
	}

	output := dst
	if !logCfg.OutputJSON {
		cl := zerolog.ConsoleWriter{
			Out:        dst,
			NoColor:    !logCfg.Color,
			TimeFormat: logCfg.TimeFormat,
		}

		if logCfg.TimeFormat == "" {
			cl.PartsExclude = []string{
				zerolog.TimestampFieldName,
			}
		}

		output = cl
	}

	if logCfg.Filter != nil {
		output = log.NewFilterWriter(output, logCfg.Filter)
	}

	logger := zerolog.New(output)
	if logCfg.StackTrace {
		zerolog.ErrorStackMarshaler = func(err error) interface{} {
			return pkgerrors.MarshalStack(perrors.WithStack(err))
		}

		logger = logger.With().Stack().Logger()
	}

	if logCfg.TimeFormat != "" {
		logger = logger.With().Timestamp().Logger()
	}

	if logCfg.Level != zerolog.NoLevel {
		logger = logger.Level(logCfg.Level)
	}

	logger = logger.Hook(logCfg.Hooks...)

	return zeroLogWrapper{&logger}
}

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

	if err := bindFlags(cmd, serverCtx.Viper, envPrefixes); err != nil {
		return err
	}

	// intercept configuration files, using both Viper instances separately
	cfg, err := interceptConfigs(serverCtx.Viper, customAppConfigTemplate, customAppConfig)
	if err != nil {
		return err
	}

	// return value is a tendermint configuration object
	serverCtx.Config = cfg

	var opts []log.Option

	logFmt := serverCtx.Viper.GetString(cflags.FlagLogFormat)
	switch logFmt {
	case tmcfg.LogFormatJSON:
		opts = append(opts, log.OutputJSONOption())
	case "":
		fallthrough
	case tmcfg.LogFormatPlain:
	// 	cl := zerolog.ConsoleWriter{
	// 		Out:        os.Stdout,
	// 		NoColor:    !serverCtx.Viper.GetBool(cflags.FlagLogColor),
	// 		TimeFormat: logTimeFmt,
	// 	}
	//
	// 	if logTimeFmt == "" {
	// 		cl.PartsExclude = []string{
	// 			zerolog.TimestampFieldName,
	// 		}
	// 	}
	// 	logWriter = cl
	default:
		return fmt.Errorf("unsupported value \"%s\" for log_format flag. can be either plain|json", logFmt)
	}

	logTimeFmt, err := parseTimestampFormat(serverCtx.Viper.GetString(cflags.FlagLogTimestamp))
	if err != nil {
		return err
	}

	opts = append(opts,
		log.ColorOption(serverCtx.Viper.GetBool(cflags.FlagLogColor)),
		log.TraceOption(serverCtx.Viper.GetBool(cflags.FlagTrace)),
		log.TimeFormatOption(logTimeFmt),
	)

	// check and set filter level or keys for the logger if any
	logLvlStr := serverCtx.Viper.GetString(cflags.FlagLogLevel)
	if logLvlStr != "" {
		logLvl, err := zerolog.ParseLevel(logLvlStr)
		switch {
		case err != nil:
			// If the log level is not a valid zerolog level, then we try to parse it as a key filter.
			filterFunc, err := log.ParseLogLevel(logLvlStr)
			if err != nil {
				return err
			}

			opts = append(opts, log.FilterOption(filterFunc))
		default:
			opts = append(opts, log.LevelOption(logLvl))
		}
	}

	logger := NewLogger(tmlog.NewSyncWriter(os.Stdout), opts...).With(log.ModuleKey, "server")

	serverCtx.Logger = logger

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

	return "", fmt.Errorf("invalid timestamp format (%s)", val)
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
	rootDir := rootViper.GetString(cflags.FlagHome)
	configPath := filepath.Join(rootDir, "config")
	tmCfgFile := filepath.Join(configPath, "config.toml")

	conf := tmcfg.DefaultConfig()

	switch _, err := os.Stat(tmCfgFile); {
	case os.IsNotExist(err):
		tmcfg.EnsureRoot(rootDir)

		if err = conf.ValidateBasic(); err != nil {
			return nil, fmt.Errorf("error in config file: %v", err)
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
			return nil, fmt.Errorf("failed to read in %s: %w", tmCfgFile, err)
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
				return nil, fmt.Errorf("failed to parse %s: %w", appCfgFilePath, err)
			}

			config.WriteConfigFile(appCfgFilePath, customConfig)
		} else {
			appConf, err := config.ParseConfig(rootViper)
			appConf.MinGasPrices = "0.025uakt"
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", appCfgFilePath, err)
			}

			config.WriteConfigFile(appCfgFilePath, appConf)
		}
	}

	rootViper.SetConfigType("toml")
	rootViper.SetConfigName("app")
	rootViper.AddConfigPath(configPath)

	if err := rootViper.MergeInConfig(); err != nil {
		return nil, fmt.Errorf("failed to merge configuration: %w", err)
	}

	return conf, nil
}
