package clitest

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
)

// This is a real CLI scan against simnode's loopback mock chains, not LOCAL evidence.
func TestLegacyPreviewScansWithoutCustody(t *testing.T) {
	for _, watch := range []bool{false, true} {
		t.Run(map[bool]string{false: "once", true: "watch"}[watch], func(t *testing.T) {
			n := startNode(t, "clean")
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			var signingRequests atomic.Int64
			go func() {
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}

					signingRequests.Add(1)
					conn.Close()
				}
			}()
			t.Setenv("AKT_REPORTER_BROADCAST", "true")
			t.Setenv("AKT_REPORTER_SIGNER_REMOTE_ADDRESS", listener.Addr().String())
			t.Setenv("AKT_REPORTER_SIGNER_REMOTE_KEY_ID", "must-not-be-read")
			t.Setenv("AKT_REPORTER_FROM", "must-not-be-loaded")
			t.Setenv("AKT_REPORTER_HOME", t.TempDir())
			input := map[string]any{
				"akashRpc":          n.CometURL + "?token=SOURCE",
				"solanaRpc":         n.SolanaURL + "?token=SECRET",
				"akashChainId":      simnode.ChainID,
				"reporter":          simnode.Reporter,
				"solanaGenesisHash": simnode.GenesisHash,
				"programId":         simnode.ProgramID.String(),
				"mint":              simnode.MintAddress.String(),
				"maxAgeSeconds":     60,
				"pollSeconds":       1,
				"gasLimit":          "200000",
				"feeAmountUakt":     "5000",
				"keyFile":           filepath.Join(t.TempDir(), "must-not-be-opened.key"),
			}

			data, err := json.Marshal(input)
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, data, 0600))
			args := []string{"run", "--config", path, "--network", "testnet"}
			if watch {
				args = append(args, "--watch")
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var stdout, stderr syncBuffer
			done := make(chan int, 1)
			go func() {
				done <- run(ctx, &stdout, &stderr, args...)
			}()
			if watch {
				require.Eventually(t, func() bool {
					return strings.Count(stdout.String(), `"scanned"`) >= 2
				}, 5*time.Second, 10*time.Millisecond)
				cancel()
			}

			select {
			case code := <-done:
				require.Equal(t, 0, code, stderr.String())
			case <-time.After(5 * time.Second):
				t.Fatal("legacy preview did not finish")
			}

			events := lines(t, stdout.String())
			if watch {
				require.GreaterOrEqual(t, summaries(events), 2)
			} else {
				require.Equal(t, 1, summaries(events))
			}

			require.NotEmpty(t, events)
			require.EqualValues(t, 5, events[len(events)-1]["scanned"])
			require.Positive(t, n.Calls("abci_query"))
			require.Positive(t, n.Calls("getGenesisHash"))
			require.Zero(t, n.Calls("broadcast_tx_sync"))
			require.Zero(t, n.Calls("/cosmos.tx.v1beta1.Service/Simulate"))
			require.Zero(t, n.Calls("/cosmos.auth.v1beta1.Query/Account"))
			require.Zero(t, signingRequests.Load())
			require.NotContains(t, stderr.String(), "SOURCE")
			require.NotContains(t, stderr.String(), "SECRET")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, data, after)
		})
	}
}
