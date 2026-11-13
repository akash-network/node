package cmd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	osring "github.com/99designs/keyring"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/codec"
	cosmoskeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

const envDBusSession = "DBUS_SESSION_BUS_ADDRESS"

var errInvalidHexKey = errors.New("the private key must be hex-encoded")

// newKeysCmd returns the cosmos-sdk keyring commands. Their own --keyring-backend and --keyring-dir shadow the root
// flags; unless set on the command line, the configured keyring-backend and keyring-dir apply.
func newKeysCmd() *cobra.Command {
	cmd := keys.Commands()

	cmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if err := setupLogger(cmd); err != nil {
			return err
		}

		return setKeysContext(cmd)
	}

	for _, sub := range cmd.Commands() {
		if sub.Name() == "import-hex" {
			sanitizeHexErrors(sub)
		}
	}

	return cmd
}

// setKeysContext builds the client.Context the keys commands read: the codec, stdin and stdout, the home directory and
// the configured keyring.
func setKeysContext(cmd *cobra.Command) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	home, err := config.ResolveHome(cmd.Flags(), os.Environ())
	if err != nil {
		return err
	}

	// The keys commands would otherwise connect to the configured node and gRPC address; they never need either.
	for _, name := range []string{flags.FlagNode, flags.FlagGRPC} {
		if err := cmd.Flags().Set(name, ""); err != nil {
			return err
		}
	}

	enc := sdkutil.MakeEncodingConfig()

	clientCtx := client.Context{}.
		WithCodec(enc.Codec).
		WithInterfaceRegistry(enc.InterfaceRegistry).
		WithTxConfig(enc.TxConfig).
		WithLegacyAmino(enc.Amino).
		WithInput(cmd.InOrStdin()).
		WithOutput(cmd.OutOrStdout()).
		WithHomeDir(home).
		WithKeyringDir(cfg.KeyringDir)

	kr, err := newKeyring(cfg.KeyringBackend, cfg.KeyringDir, clientCtx.Input, enc.Codec)
	if err != nil {
		return err
	}

	return client.SetCmdClientContextHandler(clientCtx.WithKeyring(kr), cmd)
}

// newKeyring opens the keyring of backend in dir. The os backend must reach the operating system's secret store:
// cosmos-sdk would otherwise fall back to another backend, such as an encrypted file, without a word.
func newKeyring(backend, dir string, input io.Reader, cdc codec.Codec) (cosmoskeyring.Keyring, error) {
	if backend == cosmoskeyring.BackendOS {
		if err := checkOSKeyring(); err != nil {
			return nil, err
		}
	}

	kr, err := cosmoskeyring.New(sdk.KeyringServiceName(), backend, dir, input, cdc)
	if err != nil {
		return nil, fmt.Errorf("keyring-backend %q: %w", backend, err)
	}

	return kr, nil
}

// checkOSKeyring requires the secret store the os backend opens first on this platform.
func checkOSKeyring() error {
	var allowed []osring.BackendType

	switch runtime.GOOS {
	case "darwin":
		allowed = []osring.BackendType{osring.KeychainBackend}
	case "windows":
		allowed = []osring.BackendType{osring.WinCredBackend}
	default:
		if os.Getenv(envDBusSession) == "" {
			return osKeyringError(envDBusSession + " is not set, so no Secret Service is reachable")
		}
		allowed = []osring.BackendType{osring.SecretServiceBackend, osring.KWalletBackend}
	}

	if _, err := osring.Open(osring.Config{AllowedBackends: allowed, ServiceName: sdk.KeyringServiceName()}); err != nil {
		if runtime.GOOS == "darwin" {
			return osKeyringError("the macOS keychain needs a build with CGO")
		}

		return osKeyringError(fmt.Sprintf("no %v store is reachable", allowed))
	}

	return nil
}

func osKeyringError(reason string) error {
	return fmt.Errorf(`keyring-backend "os" is unavailable: %s; use keyring-backend "file" or "test"`, reason)
}

// sanitizeHexErrors replaces hex decoding errors, which quote the offending character of the key, with a fixed text.
// It also refuses a key argument shorter than 2 characters, which cosmos-sdk slices without a bounds check.
func sanitizeHexErrors(cmd *cobra.Command) {
	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 2 && len(args[1]) < 2 {
			return errInvalidHexKey
		}

		err := run(cmd, args)

		var invalid hex.InvalidByteError
		if errors.As(err, &invalid) || errors.Is(err, hex.ErrLength) {
			return errInvalidHexKey
		}

		return err
	}
}
