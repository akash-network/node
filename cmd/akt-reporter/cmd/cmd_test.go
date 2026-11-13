package cmd_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/keyring"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/cmd/akt-reporter/cmd"
	"pkg.akt.dev/node/v2/operator/reporter/config"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

// execute runs the CLI in-process with args and stdin and returns its exit code, stdout and stderr.
func execute(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()

	root := cmd.NewRootCmd()

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	root.SetContext(context.Background())

	code := cmd.Execute(root)

	return code, stdout.String(), stderr.String()
}

func seedKey(label string) solana.PublicKey {
	sum := sha256.Sum256([]byte(label))
	return solana.PublicKeyFromBytes(sum[:])
}

// validFlags is a valid testnet preview configuration whose endpoints all point at addr.
func validFlags(t *testing.T, home, addr string) []string {
	t.Helper()

	reporter, err := bech32.ConvertAndEncode("akash", bytes.Repeat([]byte{0x42}, 20))
	require.NoError(t, err)

	program := seedKey("program")

	return []string{
		"--home", home,
		"--network", "testnet",
		"--chain-id", "test-1",
		"--reporter", reporter,
		"--node", "http://" + addr,
		"--grpc-addr", addr,
		"--solana.rpc", "http://" + addr,
		"--solana.genesis-hash", seedKey("genesis").String(),
		"--solana.program-id", program.String(),
		"--solana.mint", aktsolana.PDA(program, []byte("mint")).String(),
	}
}

func TestCheckPrintsValid(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var accepted atomic.Int64
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = c.Close()
		}
	}()

	code, stdout, stderr := execute(t, "", append([]string{"check"}, validFlags(t, t.TempDir(), l.Addr().String())...)...)
	require.NoError(t, l.Close())

	require.Equal(t, 0, code, stderr)
	require.Equal(t, "Configuration valid. No RPC request or transaction sent.\n", stdout)
	require.Zero(t, accepted.Load(), "check must not contact any endpoint")
}

func TestCheckReportsBrokenRules(t *testing.T) {
	code, stdout, stderr := execute(t, "", "check", "--home", t.TempDir())

	require.Equal(t, 1, code)
	require.Empty(t, stdout)
	require.Contains(t, stderr, "reporter: is required")
}

func TestInitWritesAndRefusesOverwrite(t *testing.T) {
	home := t.TempDir()
	path := config.FilePath(home)

	code, stdout, stderr := execute(t, "", "init", "--home", home)
	require.Equal(t, 0, code, stderr)
	require.FileExists(t, path)
	require.Contains(t, stdout, path)
	require.Contains(t, stdout, "reporter: is required")
	require.Contains(t, stdout, "solana.program-id: is required")

	data, err := os.ReadFile(path) //nolint:gosec // a test directory
	require.NoError(t, err)
	require.Contains(t, string(data), `keyring-dir = ""`, "the home directory must not be baked into the file")

	code, _, stderr = execute(t, "", "init", "--home", home, "--chain-id", "other-1")
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "--overwrite")

	again, err := os.ReadFile(path) //nolint:gosec // a test directory
	require.NoError(t, err)
	require.Equal(t, data, again, "a refused init must not touch the file")

	code, _, stderr = execute(t, "", "init", "--home", home, "--overwrite", "--chain-id", "other-1")
	require.Equal(t, 0, code, stderr)

	data, err = os.ReadFile(path) //nolint:gosec // a test directory
	require.NoError(t, err)
	require.Contains(t, string(data), `chain-id = "other-1"`)
}

func TestInitOverwriteIgnoresOldFile(t *testing.T) {
	home := t.TempDir()
	path := config.FilePath(home)

	const stale = "StaLeProgram1111111111111111111111111111111"
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("retired-key = 1\n\n[solana]\nprogram-id = \""+stale+"\"\n"), 0o600))

	t.Setenv("AKT_REPORTER_CHAIN_ID", "env-1")

	code, stdout, stderr := execute(t, "", "init", "--home", home, "--overwrite", "--network", "testnet")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, stdout, "solana.program-id: is required", "the old program ID must not satisfy the rule")

	data, err := os.ReadFile(path) //nolint:gosec // a test directory
	require.NoError(t, err)
	require.NotContains(t, string(data), "retired-key")
	require.NotContains(t, string(data), stale)
	require.Contains(t, string(data), `program-id = ""`)
	require.Contains(t, string(data), `network = "testnet"`, "flags still apply")
	require.Contains(t, string(data), `chain-id = "env-1"`, "the environment still applies")
}

func TestKeysUseConfiguredBackend(t *testing.T) {
	t.Run("configured test backend", func(t *testing.T) {
		home := t.TempDir()

		code, _, stderr := execute(t, "", "keys", "add", "r", "--home", home)
		require.Equal(t, 0, code, stderr)
		require.DirExists(t, filepath.Join(home, "keyring-test"))

		code, stdout, stderr := execute(t, "", "keys", "list", "--home", home)
		require.Equal(t, 0, code, stderr)
		require.Contains(t, stdout, "name: r")
	})

	t.Run("configured keyring-dir from the file", func(t *testing.T) {
		home, dir := t.TempDir(), t.TempDir()

		code, _, stderr := execute(t, "", "init", "--home", home, "--keyring-dir", dir)
		require.Equal(t, 0, code, stderr)

		code, _, stderr = execute(t, "", "keys", "add", "r", "--home", home)
		require.Equal(t, 0, code, stderr)
		require.DirExists(t, filepath.Join(dir, "keyring-test"))
		require.NoDirExists(t, filepath.Join(home, "keyring-test"))
	})

	t.Run("explicit file backend wins", func(t *testing.T) {
		home := t.TempDir()

		code, _, stderr := execute(t, "passphrase1\npassphrase1\n", "keys", "add", "r", "--home", home, "--keyring-backend", "file")
		require.Equal(t, 0, code, stderr)
		require.DirExists(t, filepath.Join(home, "keyring-file"))
		require.NoDirExists(t, filepath.Join(home, "keyring-test"))
	})
}

func TestOSBackendUnavailable(t *testing.T) {
	if runtime.GOOS == "darwin" && slices.Contains(keyring.AvailableBackends(), keyring.KeychainBackend) {
		t.Skip("the macOS keychain is available in a CGO build; run with CGO_ENABLED=0")
	}

	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")

	for name, args := range map[string][]string{
		"flag":        {"--keyring-backend", "os"},
		"environment": nil,
	} {
		t.Run(name, func(t *testing.T) {
			if args == nil {
				t.Setenv("AKT_REPORTER_KEYRING_BACKEND", "os")
			}

			home := t.TempDir()
			code, stdout, stderr := execute(t, "", append([]string{"keys", "list", "--home", home}, args...)...)

			require.Equal(t, 1, code)
			require.Empty(t, stdout)
			require.Contains(t, stderr, `keyring-backend "os"`)
			require.NotContains(t, stderr, "panic")
			require.NoDirExists(t, filepath.Join(home, "keyring-file"), "os must never fall back to the file backend")
		})
	}
}

func TestKeyMaterialNeverPrinted(t *testing.T) {
	t.Run("CLI refuses implicit signing and never echoes malformed private key contents", func(t *testing.T) {
		const secret = "DO_NOT_PRINT_THIS_PRIVATE_KEY_FILE_CONTENT" //nolint:gosec // the TS test's marker, not a credential

		for name, run := range map[string]func(home string) (int, string, string){
			"stdin": func(home string) (int, string, string) {
				return execute(t, secret+"\n", "keys", "import-hex", "r", "--home", home)
			},
			"argument": func(home string) (int, string, string) {
				return execute(t, "", "keys", "import-hex", "r", secret, "--home", home)
			},
		} {
			t.Run(name, func(t *testing.T) {
				code, stdout, stderr := run(t.TempDir())

				require.Equal(t, 1, code)
				require.Contains(t, stderr, "the private key must be hex-encoded")
				for _, out := range []string{stdout, stderr} {
					require.NotContains(t, out, secret)
					// The hex decoder's error quotes the offending character of the key.
					require.NotContains(t, out, "invalid byte")
					require.NotContains(t, out, "U+")
				}
			})
		}
	})
}

func TestStartStopsDuringStartup(t *testing.T) {
	// The listener accepts and never answers, so the Akash dial waits until the context ends.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(200*time.Millisecond, cancel)

	root := cmd.NewRootCmd()

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"start", "--request-timeout", "60s"}, validFlags(t, t.TempDir(), l.Addr().String())...))
	root.SetContext(ctx)

	require.Equal(t, 0, cmd.Execute(root), stderr.String())
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "stopped during startup")
}

func TestVersionOutput(t *testing.T) {
	code, stdout, stderr := execute(t, "", "version")
	require.Equal(t, 0, code, stderr)

	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	require.Len(t, lines, 5)
	require.Equal(t, "service akt-reporter", lines[0])
	require.True(t, strings.HasPrefix(lines[1], "version "), lines[1])
	require.True(t, strings.HasPrefix(lines[2], "commit "), lines[2])
	require.Equal(t, "platform "+runtime.GOOS+"_"+runtime.GOARCH, lines[3])
	require.Equal(t, "go "+runtime.Version(), lines[4])
}
