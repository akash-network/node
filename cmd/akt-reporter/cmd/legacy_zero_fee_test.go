package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

func zeroFeeCLIFile(t *testing.T, endpoint string, age, timeout int, command string) string {
	t.Helper()
	path := legacyCLIFile(t, endpoint)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var input map[string]any
	require.NoError(t, json.Unmarshal(data, &input))
	input["feeAmountUakt"] = "0"
	input["maxAgeSeconds"] = age
	if command != "" {
		delete(input, "keyFile")
		input["externalSigner"] = map[string]any{
			"command":        command,
			"timeoutSeconds": timeout,
		}
	}
	data, err = json.Marshal(input)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestLegacyZeroFeeSelection(t *testing.T) {
	path := zeroFeeCLIFile(t, "http://127.0.0.1:1", 60, 0, "")
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	for key, value := range map[string]string{
		"AKT_REPORTER_GAS_PRICES":  "0.025uakt",
		"AKT_REPORTER_FEES":        "1uakt",
		"AKT_REPORTER_GAS":         "auto",
		"AKT_REPORTER_TX_MAX_MSGS": "25",
		"AKT_REPORTER_BROADCAST":   "true",
		"AKT_REPORTER_HOME":        t.TempDir(),
	} {
		t.Setenv(key, value)
	}
	for _, broadcast := range []bool{false, true} {
		t.Run(fmt.Sprintf("broadcast=%t", broadcast), func(t *testing.T) {
			root := NewRootCmd()
			command, _, err := root.Find([]string{"run"})
			require.NoError(t, err)
			var got config.Resolved
			// Exercise selection and resolution without starting a scanner or custody.
			command.RunE = func(cmd *cobra.Command, _ []string) error {
				var err error
				got, err = resolveConfig(cmd)
				return err
			}
			args := []string{"run", "--config", path, "--network", "testnet"}
			if broadcast {
				args = append(args, "--broadcast")
			}
			root.SetArgs(args)
			require.NoError(t, root.ExecuteContext(t.Context()))
			require.Equal(t, "0uakt", got.Config.Fees)
			require.Empty(t, got.Fees)
			require.Empty(t, got.Config.GasPrices)
			require.Empty(t, got.GasPrices)
			require.Equal(t, uint64(200000), got.GasPerMessage)
			require.Equal(t, 1, got.Tx.MaxMsgs)
			require.Equal(t, broadcast, got.Broadcast)
		})
	}
	for _, command := range []string{"run", "check"} {
		for _, flag := range []string{"--gas-prices", "--fees"} {
			root := NewRootCmd()
			root.SetArgs([]string{command, "--config", path, "--network", "testnet", flag, "1uakt"})
			require.ErrorContains(t, root.ExecuteContext(t.Context()), flag+" cannot be combined with legacy --config")
		}
	}
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, after)
}

func TestLegacyZeroFeeOfflineBudget(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var requests atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			requests.Add(1)
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})

	fixture := t.TempDir()
	command := filepath.Join(fixture, "signer")
	require.NoError(t, os.WriteFile(command, []byte("#!/bin/sh\n: > \"$0.started\"\nexit 1\n"), 0o700))
	marker := command + ".started"
	for _, budget := range [][2]int{{80, 0}, {81, 0}, {70, 10}, {71, 10}, {1, 79}, {1, 80}, {600, 0}, {600, 120}} {
		for _, broadcast := range []bool{false, true} {
			t.Run(fmt.Sprintf("age%d/timeout%d/broadcast=%t", budget[0], budget[1], broadcast), func(t *testing.T) {
				selected := ""
				if budget[1] > 0 {
					selected = command
				}
				path := zeroFeeCLIFile(t, "http://"+listener.Addr().String(), budget[0], budget[1], selected)
				original, err := os.ReadFile(path)
				require.NoError(t, err)
				invalid := broadcast && budget[0]+budget[1] > 80
				commands := []string{"check"}
				if invalid {
					commands = append(commands, "run")
				}
				for _, name := range commands {
					root := NewRootCmd()
					var out bytes.Buffer
					root.SetOut(&out)
					args := []string{name, "--config", path, "--network", "testnet"}
					if broadcast {
						args = append(args, "--broadcast")
					}
					root.SetArgs(args)
					err := root.ExecuteContext(t.Context())
					if invalid {
						require.ErrorContains(t, err, "max-age:")
						require.NotContains(t, err.Error(), "fees:")
						require.Empty(t, out.String())
					} else {
						require.NoError(t, err)
						require.Equal(t, checkValid+"\n", out.String())
					}
				}
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, original, after)
				require.NoFileExists(t, marker)
				require.Zero(t, requests.Load(), "offline validation contacted RPC")
			})
		}
	}
}
