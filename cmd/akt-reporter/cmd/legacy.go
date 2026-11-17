package cmd

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

// configSelection rejects repeated selectors instead of silently choosing the last file.
type configSelection struct {
	path     string
	selected bool
}

func (s *configSelection) String() string { return s.path }

func (*configSelection) Type() string { return "path" }

func (s *configSelection) Set(path string) error {
	if s.selected {
		return errors.New("--config must be selected only once")
	}

	s.path = path
	s.selected = true
	return nil
}

func addLegacyConfigFlag(cmd *cobra.Command) {
	cmd.Flags().Var(&configSelection{}, "config", "existing packaged reporter JSON (absolute path)")
}

func newLegacyRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run an existing packaged JSON configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("config") {
				return errors.New("run requires --config")
			}

			watch, _ := cmd.Flags().GetBool("watch")
			return runReporter(cmd, !watch)
		},
	}

	addLegacyConfigFlag(cmd)
	cmd.Flags().Bool("watch", false, "repeat passes until stopped")
	return cmd
}

func loadLegacyConfig(cmd *cobra.Command) (config.Config, error) {
	var conflict error
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "config", "network", "broadcast", "watch", flagLogLevel, flagLogFormat, flagLogColor, flagLogTimestamp:
		default:
			conflict = fmt.Errorf("--%s cannot be combined with legacy --config", f.Name)
		}
	})
	if conflict != nil {
		return config.Config{}, conflict
	}

	network, _ := cmd.Flags().GetString("network")
	broadcast, _ := cmd.Flags().GetBool("broadcast")
	cfg, err := config.LoadLegacy(cmd.Flags().Lookup("config").Value.String(), network, broadcast)
	if err != nil {
		return config.Config{}, err
	}

	return cfg, nil
}

// legacyLogEndpoint omits operator routing tokens from startup logs.
func legacyLogEndpoint(cfg config.Resolved, raw string) string {
	if cfg.Legacy == nil {
		return raw
	}
	endpoint, err := url.Parse(raw)
	if err != nil {
		return "legacy RPC"
	}
	return endpoint.Scheme + "://" + endpoint.Host
}
