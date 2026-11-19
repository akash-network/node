package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/config"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

func legacyCLIFile(t *testing.T, endpoint string) string {
	t.Helper()
	address, err := bech32.ConvertAndEncode("akash", bytes.Repeat([]byte{7}, 20))
	require.NoError(t, err)
	cfg := map[string]any{
		"akashRpc":          endpoint,
		"solanaRpc":         endpoint,
		"akashChainId":      "r176-test",
		"reporter":          address,
		"solanaGenesisHash": solana.TokenProgramID.String(),
		"programId":         solana.TokenProgramID.String(),
		"mint":              aktsolana.PDA(solana.TokenProgramID, []byte("mint")).String(),
		"maxAgeSeconds":     60,
		"pollSeconds":       3,
		"gasLimit":          "200000",
		"feeAmountUakt":     "5000",
		"keyFile":           filepath.Join(t.TempDir(), "does-not-exist.key"),
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

func TestLegacyPackagedCheck(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	var requests atomic.Int64
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			requests.Add(1)
			conn.Close()
		}
	}()
	path := legacyCLIFile(t, "http://"+listener.Addr().String()+"/rpc?tag=r176")
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, broadcast := range []bool{false, true} {
		root := NewRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		args := []string{"check", "--config", path, "--network", "testnet"}
		if broadcast {
			args = append(args, "--broadcast")
		}

		root.SetArgs(args)
		require.NoError(t, root.ExecuteContext(context.Background()))
		require.Equal(t, checkValid+"\n", out.String())
	}

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(original, after))
	require.Zero(t, requests.Load(), "offline check accessed RPC")
}

func TestLegacyPackagedRunSelection(t *testing.T) {
	path := legacyCLIFile(t, "http://127.0.0.1:1/rpc?tag=r176")
	t.Setenv("AKT_REPORTER_BROADCAST", "true")
	t.Setenv("AKT_REPORTER_FROM", "injected")
	t.Setenv("AKT_REPORTER_SIGNER_REMOTE_ADDRESS", "127.0.0.1:1")
	t.Setenv("AKT_REPORTER_HOME", t.TempDir())
	for _, watch := range []bool{false, true} {
		for _, broadcast := range []bool{false, true} {
			root := NewRootCmd()
			command, _, err := root.Find([]string{"run"})
			require.NoError(t, err)
			require.Equal(t, "run", command.Name())
			// Exercise the packaged argument and resolution boundary without starting a scan.
			var resolved config.Resolved
			command.RunE = func(cmd *cobra.Command, _ []string) error {
				var err error
				resolved, err = resolveConfig(cmd)
				return err
			}

			args := []string{"run", "--config", path, "--network", "testnet"}
			if watch {
				args = append(args, "--watch")
			}

			if broadcast {
				args = append(args, "--broadcast")
			}

			root.SetArgs(args)
			require.NoError(t, root.ExecuteContext(context.Background()))
			selected, err := command.Flags().GetBool("watch")
			require.NoError(t, err)
			require.Equal(t, watch, selected)
			require.Equal(t, broadcast, resolved.Broadcast)
			require.Equal(t, "5000uakt", resolved.Fees.String())
			require.Empty(t, resolved.GasPrices)
			require.Equal(t, 1, resolved.Tx.MaxMsgs)
			require.Empty(t, resolved.From)
			require.Empty(t, resolved.Signer.Remote.Address)
			require.Empty(t, resolved.GRPCAddr)
		}
	}
}

func TestLegacySelectionConflicts(t *testing.T) {
	path := legacyCLIFile(t, "http://127.0.0.1:1")
	for _, extra := range [][]string{
		{"--config", path},
		{"--from", "another"},
		{"--signer.remote.address", "127.0.0.1:1"},
		{"--grpc-addr", "127.0.0.1:1"},
		{"--home", t.TempDir()},
		{"--fees", "1uakt"},
		{"--key-file", "/ignored.key"},
		{"--signer-command", "/ignored-command"},
		{"--watch"},
	} {
		root := NewRootCmd()
		root.SetArgs(append([]string{"check", "--config", path, "--network", "testnet"}, extra...))
		require.Error(t, root.ExecuteContext(context.Background()))
	}

	root := NewRootCmd()
	root.SetArgs([]string{"run"})
	require.ErrorContains(t, root.ExecuteContext(context.Background()), "requires --config")
}
