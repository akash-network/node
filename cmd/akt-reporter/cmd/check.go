package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

const checkValid = "Configuration valid. No RPC request or transaction sent."

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate the configuration offline, without RPC, keyring or signer access",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := resolveConfig(cmd); err != nil {
				return err
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), checkValid)

			return err
		},
	}
}

// resolveConfig loads and validates the configuration, as check, start and scan do first (spec §8.4).
func resolveConfig(cmd *cobra.Command) (config.Resolved, error) {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return config.Resolved{}, err
	}

	res, err := config.Resolve(cfg)
	if err != nil {
		return config.Resolved{}, fmt.Errorf("invalid configuration:\n%w", err)
	}

	return res, nil
}
