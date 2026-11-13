package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

const flagOverwrite = "overwrite"

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write <home>/config/reporter.toml from the defaults, flags and environment",
		Args:  cobra.NoArgs,
		RunE:  runInit,
	}

	cmd.Flags().Bool(flagOverwrite, false, "replace an existing configuration file")

	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	overwrite, _ := cmd.Flags().GetBool(flagOverwrite)

	home, err := config.ResolveHome(cmd.Flags(), os.Environ())
	if err != nil {
		return err
	}

	path := config.FilePath(home)
	if !overwrite {
		if _, err := os.Lstat(path); err == nil {
			return errExists(path)
		}
	}

	// init renders defaults, flags and environment only; the file it may replace is never read (spec §8.2).
	cfg, err := loadConfigWith(cmd, config.LoadWithoutFile)
	if err != nil {
		return err
	}

	// An empty keyring-dir already means the home directory; keep the home path out of the file.
	if cfg.KeyringDir == home {
		cfg.KeyringDir = ""
	}

	data, err := config.Render(cfg)
	if err != nil {
		return err
	}

	if err := writeConfig(path, data, overwrite); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Wrote %s\n", path); err != nil {
		return err
	}

	// A fresh file leaves the required keys empty, so it is written even when it breaks rules (spec §8.4).
	if _, err := config.Resolve(cfg); err != nil {
		return printRules(out, err)
	}

	return nil
}

func errExists(path string) error {
	return fmt.Errorf("%s already exists; use --%s to replace it", path, flagOverwrite)
}

func writeConfig(path string, data []byte, overwrite bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	mode := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwrite {
		mode = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	f, err := os.OpenFile(path, mode, 0o600) //nolint:gosec // the operator names the home directory
	if errors.Is(err, fs.ErrExist) {
		return errExists(path)
	}
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}

// printRules lists every rule err reports as broken, one per line.
func printRules(out io.Writer, err error) error {
	rules := []error{err}

	var joined interface{ Unwrap() []error }
	if errors.As(err, &joined) {
		rules = joined.Unwrap()
	}

	if _, err := fmt.Fprintln(out, "The configuration does not yet satisfy these rules:"); err != nil {
		return err
	}

	for _, rule := range rules {
		if _, err := fmt.Fprintf(out, "  %s\n", rule); err != nil {
			return err
		}
	}

	return nil
}
