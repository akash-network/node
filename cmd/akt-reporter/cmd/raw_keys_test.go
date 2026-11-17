package cmd_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/cmd/akt-reporter/cmd"
	"pkg.akt.dev/node/v2/operator/reporter/config"
)

func rawCLIHome(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	home := filepath.Join(dir, "uncreated-home")
	t.Setenv("HOME", home)
	t.Setenv("AKT_REPORTER_HOME", home)
	t.Setenv("AKT_REPORTER_KEYRING_BACKEND", "invalid-do-not-open")
	t.Setenv("AKT_REPORTER_REQUEST_TIMEOUT", "invalid-do-not-parse")
	t.Cleanup(func() { require.NoDirExists(t, home) })

	return dir
}

func TestRawKeyCLICreate(t *testing.T) {
	dir := rawCLIHome(t)
	path := filepath.Join(dir, "generated.key")
	code, stdout, stderr := execute(t, "", "keys", "create", "--out", path)
	require.Equal(t, 0, code, stderr)
	require.Empty(t, stderr)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	defer clear(data)
	require.Len(t, data, 65)
	require.Equal(t, byte('\n'), data[64])
	require.True(t, strings.Trim(string(data[:64]), "0123456789abcdef") == "", "key format")
	decoded, err := hex.DecodeString(string(data[:64]))
	require.NoError(t, err)
	defer clear(decoded)
	key := &sdksecp.PrivKey{Key: decoded}
	address := sdk.AccAddress(key.PubKey().Address()).String()
	require.Equal(t, address+"\n", stdout)
	require.False(t, strings.Contains(stdout+stderr, string(data[:64])), "private key leaked")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	code, stdout, stderr = execute(t, "", "keys", "address", "--key-file", path)
	require.Equal(t, 0, code, stderr)
	require.Equal(t, address+"\n", stdout)
	require.Empty(t, stderr)

	for _, target := range []string{path, path + ".link", path + ".dangling"} {
		if target == path+".link" {
			require.NoError(t, os.Symlink(path, target))
		}
		if target == path+".dangling" {
			require.NoError(t, os.Symlink(path+".absent", target))
		}
		code, stdout, stderr = execute(t, "", "keys", "create", "--out", target)
		require.Equal(t, 1, code)
		require.Empty(t, stdout)
		require.NotEmpty(t, stderr)
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.True(t, bytes.Equal(data, after), "existing key changed")
		clear(after)
	}
	require.NoFileExists(t, path+".absent")
}

func TestRawKeyCLIAddress(t *testing.T) {
	dir := rawCLIHome(t)
	key := sdksecp.GenPrivKeyFromSecret([]byte("R199 task-owned disposable address fixture"))
	defer clear(key.Key)
	path := filepath.Join(dir, "address.key")
	data := []byte("\ufeff\u00a0" + hex.EncodeToString(key.Key) + "\u2028\ufeff\n")
	defer clear(data)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	code, stdout, stderr := execute(t, "", "keys", "address", "--key-file", path)
	require.Equal(t, 0, code, stderr)
	require.Equal(t, sdk.AccAddress(key.PubKey().Address()).String()+"\n", stdout)
	require.Empty(t, stderr)
}

func TestRawKeyCLIArguments(t *testing.T) {
	dir := rawCLIHome(t)
	path := filepath.Join(dir, "must-not-exist.key")
	cases := map[string][]string{
		"create missing":     {"keys", "create"},
		"create empty":       {"keys", "create", "--out", ""},
		"create relative":    {"keys", "create", "--out", "relative.key"},
		"create extra":       {"keys", "create", "--out", path, "extra"},
		"create opposite":    {"keys", "create", "--out", path, "--key-file", path},
		"create broadcast":   {"keys", "create", "--out", path, "--broadcast"},
		"create network":     {"keys", "create", "--out", path, "--network", "invalid"},
		"create config":      {"keys", "create", "--out", path, "--config", filepath.Join(dir, "absent.json")},
		"create watch":       {"keys", "create", "--out", path, "--watch"},
		"create reset":       {"keys", "create", "--out", path, "--reset-budget", "all"},
		"create developer":   {"keys", "create", "--out", path, "--signer-command", "must-not-run"},
		"create missing dir": {"keys", "create", "--out", filepath.Join(dir, "absent", "key")},
		"create logger":      {"keys", "create", "--out", path, "--log_level", "invalid"},
		"address missing":    {"keys", "address"},
		"address relative":   {"keys", "address", "--key-file", "relative.key"},
		"address empty":      {"keys", "address", "--key-file", ""},
		"address extra":      {"keys", "address", "--key-file", path, "extra"},
		"address opposite":   {"keys", "address", "--key-file", path, "--out", path},
		"address absent":     {"keys", "address", "--key-file", path},
		"address broadcast":  {"keys", "address", "--key-file", path, "--broadcast"},
		"address config":     {"keys", "address", "--key-file", path, "--config", filepath.Join(dir, "absent.json")},
		"address logger":     {"keys", "address", "--key-file", path, "--log_format", "invalid"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := execute(t, "", args...)
			require.Equal(t, 1, code)
			require.Empty(t, stdout)
			require.NotEmpty(t, stderr)
			require.NoFileExists(t, path)
		})
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRawKeyCLIIgnoresTOMLAndKeepsCanonicalSetup(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "poison-home")
	configPath := config.FilePath(home)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o700))
	require.NoError(t, os.WriteFile(configPath, []byte("invalid TOML ["), 0o600))
	t.Setenv("AKT_REPORTER_HOME", home)
	path := filepath.Join(dir, "raw.key")

	for _, args := range [][]string{
		{"keys", "create", "--out", path},
		{"keys", "address", "--key-file", path},
		{"--version"},
		{"version"},
	} {
		code, _, stderr := execute(t, "", args...)
		require.Equal(t, 0, code, stderr)
		require.Empty(t, stderr)
	}

	code, stdout, stderr := execute(t, "", "keys", "list")
	require.Equal(t, 1, code, "canonical keys still load configured TOML")
	require.Empty(t, stdout)
	require.NotEmpty(t, stderr)
	require.NoDirExists(t, filepath.Join(home, "keyring-test"))
}

func TestRawKeyCLIUnsafeFiles(t *testing.T) {
	dir := rawCLIHome(t)
	path := filepath.Join(dir, "key")
	key := sdksecp.GenPrivKeyFromSecret([]byte("R199 custody fixture"))
	defer clear(key.Key)
	data := []byte(hex.EncodeToString(key.Key) + "\n")
	defer clear(data)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	require.NoError(t, os.Symlink(path, path+".link"))
	require.NoError(t, syscall.Mkfifo(path+".fifo", 0o600))

	for _, target := range []string{path + ".link", path + ".fifo", dir} {
		code, stdout, stderr := execute(t, "", "keys", "address", "--key-file", target)
		require.Equal(t, 1, code)
		require.Empty(t, stdout)
		require.NotEmpty(t, stderr)
	}

	for _, mode := range []os.FileMode{0o640, 0o604, 0o666} {
		require.NoError(t, os.Chmod(path, mode))
		code, stdout, stderr := execute(t, "", "keys", "address", "--key-file", path)
		require.Equal(t, 1, code)
		require.Empty(t, stdout)
		require.Contains(t, stderr, "owner-only regular file")
		require.False(t, strings.Contains(stderr, string(data[:64])), "private key leaked")
	}
}

type rawKeyOutputFailure struct{}

func (rawKeyOutputFailure) Write([]byte) (int, error) {
	return 0, errors.New("test output failure")
}

func TestRawKeyCLIOutputFailure(t *testing.T) {
	dir := rawCLIHome(t)
	path := filepath.Join(dir, "written.key")
	for _, args := range [][]string{
		{"keys", "create", "--out", path},
		{"keys", "address", "--key-file", path},
		{"--version"},
		{"version"},
	} {
		root := cmd.NewRootCmd()
		root.SetOut(rawKeyOutputFailure{})
		var stderr bytes.Buffer
		root.SetErr(&stderr)
		root.SetArgs(args)
		root.SetContext(t.Context())
		require.Equal(t, 1, cmd.Execute(root))
		require.Contains(t, stderr.String(), "test output failure")
	}

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, int64(65), info.Size(), "stdout failure must not remove a completed key")
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
