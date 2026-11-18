package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

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

// writeFileData writes every reporter.toml; only tests replace it.
var writeFileData = (*os.File).Write

// keepACL gives a temporary file the access ACL of the reporter.toml at path that it replaces; only tests replace it.
var keepACL = keepAccessACL

func writeConfig(path string, data []byte, overwrite bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	if overwrite {
		return replaceConfig(path, data)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the operator names the home directory
	if errors.Is(err, fs.ErrExist) {
		return errExists(path)
	}
	if err != nil {
		return err
	}

	return writeSynced(f, data)
}

// replaceConfig renames a synced temporary file over path, or over the target of a symlink at path, so a
// failed write keeps the old file; the temporary file takes the old file's mode, on Linux its POSIX access ACL, and,
// where it may, its owner and group. It keeps the old file when that ACL cannot be copied, and refuses a
// target that is not a regular file. It writes in place where no temporary file can be created or renamed into
// place, as with a single-file bind mount, and through a dangling symlink, whose target O_CREATE creates.
func replaceConfig(path string, data []byte) (err error) {
	var old fs.FileInfo

	target, err := filepath.EvalSymlinks(path)
	switch {
	case err == nil:
		path = target
		if old, err = os.Stat(path); err != nil {
			return err
		}
		if !old.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	case isSymlink(path):
		return writeInPlace(path, data)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".reporter.toml.*")
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) {
		return writeInPlace(path, data)
	}
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()

	if old != nil {
		keepOwnerAndMode(tmp, old)
		if err = keepACL(tmp, path); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("copy the access ACL of %s: %w", path, err)
		}
	}

	if err = writeSynced(tmp, data); err != nil {
		return err
	}

	err = os.Rename(tmp.Name(), path)
	if errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.EXDEV) {
		_ = os.Remove(tmp.Name())
		return writeInPlace(path, data)
	}

	return err
}

// isSymlink reports whether path itself is a symbolic link.
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&fs.ModeSymlink != 0
}

// keepOwnerAndMode gives f the owner and permission bits of the file old describes. Both are best effort: keeping
// another user's ownership takes privilege, and without it f stays owned by the user who runs init and keeps the
// old group where that user may set it.
func keepOwnerAndMode(f *os.File, old fs.FileInfo) {
	if uid, gid, ok := fileOwner(old); ok {
		if f.Chown(uid, gid) != nil {
			_ = f.Chown(-1, gid)
		}
	}
	_ = f.Chmod(old.Mode().Perm())
}

// writeInPlace truncates path and writes data into it, so an existing file keeps its mode and owner; a missing
// file is created 0600.
func writeInPlace(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // the operator names the home directory
	if err != nil {
		return err
	}

	return writeSynced(f, data)
}

// writeSynced writes data to f, syncs it and closes f.
func writeSynced(f *os.File, data []byte) error {
	_, err := writeFileData(f, data)
	if err == nil {
		err = f.Sync()
	}

	return errors.Join(err, f.Close())
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
