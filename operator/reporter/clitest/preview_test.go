// Package clitest runs the akt-reporter CLI in-process against the chain simulator.
package clitest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/cmd/akt-reporter/cmd"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
)

// syncBuffer is a bytes.Buffer safe to read while the CLI writes it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

type result struct {
	code           int
	stdout, stderr string
}

// run executes the CLI under ctx, writing to stdout and stderr.
func run(ctx context.Context, stdout, stderr io.Writer, args ...string) int {
	root := cmd.NewRootCmd()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	root.SetContext(ctx)

	return cmd.Execute(root)
}

func runToEnd(t *testing.T, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := run(t.Context(), &stdout, &stderr, args...)

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func startNode(t *testing.T, scenario string) *simnode.Node {
	t.Helper()

	sc, ok := simnode.Scenarios()[scenario]
	require.True(t, ok, scenario)

	n, err := simnode.Start(t.Context(), sc(), simnode.Options{})
	require.NoError(t, err)
	t.Cleanup(n.Close)

	return n
}

// command returns the arguments of a CLI command with extra arguments, configured as a preview reporter against n.
func command(t *testing.T, n *simnode.Node, name string, extra ...string) []string {
	t.Helper()

	return append(append([]string{name}, extra...),
		"--home", t.TempDir(),
		"--network", "testnet",
		"--chain-id", simnode.ChainID,
		"--reporter", simnode.Reporter,
		"--node", n.CometURL,
		"--grpc-addr", n.GRPCAddr,
		"--solana.rpc", n.SolanaURL,
		"--solana.genesis-hash", simnode.GenesisHash,
		"--solana.program-id", simnode.ProgramID.String(),
		"--solana.mint", simnode.MintAddress.String(),
		"--request-timeout", "5s",
	)
}

// lines decodes every stdout line as a JSON object.
func lines(t *testing.T, stdout string) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		if line == "" {
			continue
		}

		var v map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &v), "stdout line is not JSON: %q", line)
		out = append(out, v)
	}

	return out
}

func summaries(events []map[string]any) int {
	n := 0
	for _, ev := range events {
		if _, ok := ev["scanned"]; ok {
			n++
		}
	}

	return n
}

func TestScanExitCodes(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		n := startNode(t, "clean")

		r := runToEnd(t, command(t, n, "scan")...)
		require.Equal(t, 0, r.code, r.stderr)

		events := lines(t, r.stdout)
		require.NotEmpty(t, events)
		last := events[len(events)-1]
		require.EqualValues(t, 5, last["scanned"])
		require.EqualValues(t, 0, last["failures"])
		require.Equal(t, 1, summaries(events))
	})

	t.Run("stale-akash", func(t *testing.T) {
		n := startNode(t, "stale-akash")

		r := runToEnd(t, command(t, n, "scan")...)
		require.Equal(t, 1, r.code, r.stderr)

		events := lines(t, r.stdout)
		require.Len(t, events, 1)
		require.Contains(t, events[0], "error")
		require.Zero(t, summaries(events))
	})
}

func TestScanPinMismatch(t *testing.T) {
	n := startNode(t, "clean")

	// The bridge names simnode.LightClient; pin another program.
	pinned := simnode.ProgramID.String()
	r := runToEnd(t, command(t, n, "scan", "--solana.light-client-program", pinned)...)
	require.Equal(t, 1, r.code, r.stderr)

	events := lines(t, r.stdout)
	require.Len(t, events, 2, r.stdout)
	require.Equal(t, "light-client-mismatch", events[0]["alert"])
	require.Equal(t, pinned, events[0]["pinned"])
	require.Equal(t, simnode.LightClient.String(), events[0]["current"])
	require.Contains(t, events[1], "error")
	require.Zero(t, summaries(events))
}

func TestStartStopsOnCancel(t *testing.T) {
	n := startNode(t, "clean")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, &stdout, &stderr, command(t, n, "start", "--scan.interval", "60s")...)
	}()

	require.Eventually(t, func() bool {
		return strings.Contains(stdout.String(), `"scanned"`)
	}, 30*time.Second, 20*time.Millisecond, "no summary: %s", stderr.String())

	cancel()

	select {
	case code := <-done:
		require.Equal(t, 0, code, stderr.String())
	case <-time.After(30 * time.Second):
		t.Fatal("start did not stop after cancellation")
	}

	require.Equal(t, 1, summaries(lines(t, stdout.String())))
}

func TestLogsOnStderrEventsOnStdout(t *testing.T) {
	n := startNode(t, "clean")

	r := runToEnd(t, command(t, n, "scan", "--log_format", "json")...)
	require.Equal(t, 0, r.code, r.stderr)

	events := lines(t, r.stdout)
	require.NotEmpty(t, events)
	for _, ev := range events {
		require.NotContains(t, ev, "level", "a log line reached stdout")
	}

	require.Contains(t, r.stderr, "starting reporter")
	require.Contains(t, r.stderr, `"mode":"preview"`)
	require.NotContains(t, r.stderr, `"scanned"`, "an event reached stderr")
}

func TestMetricsInPreview(t *testing.T) {
	n := startNode(t, "clean")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, &stdout, &stderr, command(t, n, "start", "--scan.interval", "60s", "--metrics.listen", addr)...)
	}()

	require.Eventually(t, func() bool {
		return strings.Contains(stdout.String(), `"scanned"`)
	}, 30*time.Second, 20*time.Millisecond, "no summary: %s", stderr.String())

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/metrics", nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, res.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Contains(t, string(body), "akt_reporter_pass_duration_seconds_count 1")
	require.Contains(t, string(body), `akt_reporter_rpc_duration_seconds_count{chain="solana"`)
	require.Contains(t, string(body), `akt_reporter_rpc_duration_seconds_count{chain="akash"`)

	cancel()
	require.Equal(t, 0, <-done, stderr.String())
}
