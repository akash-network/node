// Package cmd is the akt-reporter command line (spec §8).
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	// Registers the akash Bech32 prefixes.
	_ "pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

const (
	serviceName = "akt-reporter"

	flagLogLevel     = "log_level"
	flagLogFormat    = "log_format"
	flagLogColor     = "log_color"
	flagLogTimestamp = "log_timestamp"

	logFormatPlain = "plain"
	logFormatJSON  = "json"
)

// exitCode ends a command with a code and no further message; the command has already reported why.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit code %d", int(c)) }

// NewRootCmd returns the akt-reporter command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   serviceName,
		Short: "Akash sink reporter",
		Long: `akt-reporter reviews open Akash sink transfers against finalized Solana state and, in broadcast mode,
votes, vetoes and refunds on Akash. Events go to stdout as JSON lines; logs go to stderr.`,
		SilenceErrors:     true,
		SilenceUsage:      true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error { return setupLogger(cmd) },
	}

	pf := root.PersistentFlags()
	pf.String(config.FlagHome, "", "home directory (default $HOME/.akt-reporter; env "+config.EnvPrefix+"_HOME)")
	pf.String(flagLogLevel, zerolog.InfoLevel.String(), "The logging level (trace|debug|info|warn|error|fatal|panic)")
	pf.String(flagLogFormat, logFormatPlain, "The logging format (json|plain)")
	pf.Bool(flagLogColor, false, "Pretty logging output. Applied only when log_format=plain")
	pf.String(flagLogTimestamp, "", "Add timestamp prefix to the logs (rfc3339|rfc3339nano|kitchen)")
	config.AddFlags(pf)

	root.AddCommand(
		newInitCmd(),
		newStartCmd(),
		newScanCmd(),
		newCheckCmd(),
		newKeysCmd(),
		newVersionCmd(),
	)

	return root
}

// Execute runs root and returns the process exit code. SIGINT and SIGTERM cancel the command's context; a second
// signal exits at once (spec §5.7).
func Execute(root *cobra.Command) int {
	ctx := root.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, stop := notifyContext(ctx, root.ErrOrStderr())
	defer stop()

	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}

	var code exitCode
	if errors.As(err, &code) {
		return int(code)
	}

	_, _ = fmt.Fprintln(root.ErrOrStderr(), "Error:", err)

	return 1
}

// notifyContext returns a context cancelled by the first SIGINT or SIGTERM. A second signal exits immediately.
func notifyContext(parent context.Context, stderr io.Writer) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
			cancel()
		case <-done:
			return
		}

		select {
		case <-signals:
			_, _ = fmt.Fprintln(stderr, "second signal received, exiting immediately")
			os.Exit(1)
		case <-done:
		}
	}()

	return ctx, func() {
		signal.Stop(signals)
		close(done)
		cancel()
	}
}

// setupLogger builds the akash-style zerolog logger on stderr from the log flags and stores it in the command's
// context.
func setupLogger(cmd *cobra.Command) error {
	flags := cmd.Flags()

	format, _ := flags.GetString(flagLogFormat)
	color, _ := flags.GetBool(flagLogColor)
	levelName, _ := flags.GetString(flagLogLevel)
	stamp, _ := flags.GetString(flagLogTimestamp)

	level, err := zerolog.ParseLevel(levelName)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", flagLogLevel, levelName, err)
	}

	timeFormat, err := parseTimestampFormat(stamp)
	if err != nil {
		return err
	}

	out := cmd.ErrOrStderr()

	switch format {
	case logFormatJSON:
	case logFormatPlain, "":
		cw := zerolog.ConsoleWriter{Out: out, NoColor: !color, TimeFormat: timeFormat}
		if timeFormat == "" {
			cw.PartsExclude = []string{zerolog.TimestampFieldName}
		}
		out = cw
	default:
		return fmt.Errorf("unsupported value %q for %s flag. can be either plain|json", format, flagLogFormat)
	}

	logger := zerolog.New(out).Level(level)
	if timeFormat != "" {
		logger = logger.With().Timestamp().Logger()
	}

	cmd.SetContext(logger.WithContext(cmd.Context()))

	return nil
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

// loadConfig loads the configuration of cmd and logs the unknown environment variables it ignored.
func loadConfig(cmd *cobra.Command) (config.Config, error) {
	return loadConfigWith(cmd, config.Load)
}

// loadConfigWith loads the configuration of cmd with load and logs the unknown environment variables it ignored.
func loadConfigWith(
	cmd *cobra.Command,
	load func(*pflag.FlagSet, []string) (config.Config, []string, error),
) (config.Config, error) {
	cfg, warnings, err := load(cmd.Flags(), os.Environ())
	if err != nil {
		return config.Config{}, err
	}

	logger := zerolog.Ctx(cmd.Context())
	for _, w := range warnings {
		logger.Warn().Msg(w)
	}

	return cfg, nil
}
