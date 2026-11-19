package scan_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
)

// The steady-100k benchmark of spec §11.4.
const (
	steadyScenario = "steady-100k"
	steadyPassMax  = 60 * time.Second
	steadyHeapMax  = 512 << 20
	heapMetric     = "/memory/classes/heap/objects:bytes"
	heapSampling   = 100 * time.Millisecond
	reportOutcome  = "/akash.sink.v1.MsgReportOutcome"
)

// TestSteady100k runs one preview pass over the 100,000 open transfers of spec §11.4 and gates its time, its peak heap
// and its decisions. simnode serves the workload from a subprocess, so the heap measured is the reporter's alone.
func TestSteady100k(t *testing.T) {
	if os.Getenv("REPORTER_PERF") != "1" {
		t.Skip("set REPORTER_PERF=1 to run the steady-100k benchmark")
	}

	counts := &eventCounts{messages: map[string]int{}, alerts: map[string]int{}, passAlerts: map[string]int{}}
	ev := events.NewWriter(counts)
	scanner := newPreviewScanner(t, serveSteady(t), ev)

	stopProfile := startCPUProfile(t)
	runtime.GC()
	sampler := sampleHeap()

	began := time.Now()
	failures := scanner.Pass(t.Context(), nil)
	elapsed := time.Since(began)

	peak := sampler.stop()
	stopProfile()
	writeHeapProfile(t)
	require.NoError(t, ev.Close())

	t.Logf("pass %v, peak heap %.1f MiB; %d reviews, %d audit early exits, %d errors, messages %v, alerts %v",
		elapsed, float64(peak)/(1<<20), counts.reviews, counts.earlyExits, counts.errors, counts.messages, counts.alerts)

	require.LessOrEqual(t, elapsed, steadyPassMax, "pass time")
	require.LessOrEqual(t, peak, uint64(steadyHeapMax), "peak heap")

	require.Zero(t, failures)
	require.Equal(t, 1, counts.summaries)
	require.Equal(t, 100_000, counts.scanned)
	require.Equal(t, 100, counts.audited)
	require.Zero(t, counts.failures)

	require.Equal(t, 100_000, counts.reviews)
	require.Equal(t, 100, counts.earlyExits)
	require.Zero(t, counts.errors)
	require.Equal(t, map[string]int{reportOutcome: 5_000}, counts.messages)
	require.Equal(t, map[string]int{policy.AlertGovernanceReview: 1_000}, counts.alerts)
	require.Empty(t, counts.passAlerts)
	require.Zero(t, counts.other)
}

// BenchmarkPass runs one preview pass over steady-100k per iteration, against the same simnode.
func BenchmarkPass(b *testing.B) {
	ev := events.NewWriter(io.Discard)
	scanner := newPreviewScanner(b, serveSteady(b), ev)
	b.Cleanup(func() { _ = ev.Close() })

	for b.Loop() {
		require.Zero(b, scanner.Pass(b.Context(), nil))
	}
}

// serveSteady builds simnode and serves steady-100k from it, with the latencies of spec §11.4, until the test ends.
func serveSteady(tb testing.TB) previewNode {
	tb.Helper()

	bin := filepath.Join(tb.TempDir(), "simnode")
	build := exec.CommandContext(tb.Context(), "go", "build", "-o", bin, "../internal/simnode/cmd/simnode") //nolint:gosec // a temp path
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	out, err := build.CombinedOutput()
	require.NoError(tb, err, "build simnode: %s", out)

	cmd := exec.Command(bin, "serve", "--scenario", steadyScenario, //nolint:gosec // the binary built above
		"--akash-latency", "2ms", "--solana-latency", "5ms", "--block-fetch-latency", "200ms")
	cmd.Stderr = os.Stderr
	// simnode serves until its stdin closes, so it must stay open.
	stdin, err := cmd.StdinPipe()
	require.NoError(tb, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(tb, err)
	require.NoError(tb, cmd.Start())
	tb.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	require.NoError(tb, err, "simnode ready line")

	var ready struct {
		Comet  string `json:"comet"`
		GRPC   string `json:"grpc"`
		Solana string `json:"solana"`
	}
	require.NoError(tb, json.Unmarshal(line, &ready))

	return previewNode{cometURL: ready.Comet, grpcAddr: ready.GRPC, solanaURL: ready.Solana}
}

// eventCounts counts a pass's events line by line as the events writer writes them, without keeping them.
type eventCounts struct {
	mu      sync.Mutex
	partial []byte

	summaries, scanned, audited, failures int
	// reviews counts the full reviews, earlyExits the audit early exits and errors the transfer errors.
	reviews, earlyExits, errors int
	// messages counts the planned messages by type URL and alerts the review alerts by text.
	messages, alerts map[string]int
	// passAlerts counts the alert events by kind; other counts lines of no known event.
	passAlerts map[string]int
	other      int
}

// eventLine holds the fields that tell the events apart.
type eventLine struct {
	ID       *string         `json:"id"`
	Mode     *string         `json:"mode"`
	Audit    bool            `json:"audit"`
	Observed json.RawMessage `json:"observed"`
	Messages []struct {
		TypeURL string `json:"typeUrl"`
	} `json:"messages"`
	Alerts   []string `json:"alerts"`
	Alert    *string  `json:"alert"`
	Error    *string  `json:"error"`
	Scanned  *int     `json:"scanned"`
	Audited  int      `json:"audited"`
	Failures int      `json:"failures"`
}

func (c *eventCounts) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.partial = append(c.partial, p...)
	for {
		i := bytes.IndexByte(c.partial, '\n')
		if i < 0 {
			break
		}

		if err := c.count(c.partial[:i]); err != nil {
			return 0, err
		}
		c.partial = c.partial[i+1:]
	}
	// Drop the consumed prefix so the buffer stays one line long.
	c.partial = append(c.partial[:0:0], c.partial...)

	return len(p), nil
}

func (c *eventCounts) count(raw []byte) error {
	var e eventLine
	if err := json.Unmarshal(raw, &e); err != nil {
		return err
	}

	switch {
	case e.Scanned != nil:
		c.summaries++
		c.scanned, c.audited, c.failures = *e.Scanned, e.Audited, e.Failures
	case e.Alert != nil:
		c.passAlerts[*e.Alert]++
	case e.ID != nil && e.Error != nil:
		c.errors++
	case e.ID != nil && e.Mode == nil && e.Audit && string(e.Observed) == `"resolved"`:
		c.earlyExits++
	case e.ID != nil && e.Mode != nil:
		c.reviews++
		for _, m := range e.Messages {
			c.messages[m.TypeURL]++
		}
		for _, a := range e.Alerts {
			c.alerts[a]++
		}
	default:
		c.other++
	}

	return nil
}

// heapSampler records the peak of heapMetric, sampled every heapSampling.
type heapSampler struct {
	done chan struct{}
	peak chan uint64
}

func sampleHeap() *heapSampler {
	s := &heapSampler{done: make(chan struct{}), peak: make(chan uint64)}

	go func() {
		sample := []metrics.Sample{{Name: heapMetric}}
		read := func(peak uint64) uint64 {
			metrics.Read(sample)
			return max(peak, sample[0].Value.Uint64())
		}

		tick := time.NewTicker(heapSampling)
		defer tick.Stop()

		peak := read(0)
		for {
			select {
			case <-tick.C:
				peak = read(peak)
			case <-s.done:
				s.peak <- read(peak)
				return
			}
		}
	}()

	return s
}

func (s *heapSampler) stop() uint64 {
	close(s.done)
	return <-s.peak
}

// startCPUProfile writes cpu.pprof to $REPORTER_PERF_OUT, when set, until the returned function runs.
func startCPUProfile(t *testing.T) func() {
	t.Helper()

	dir := os.Getenv("REPORTER_PERF_OUT")
	if dir == "" {
		return func() {}
	}

	require.NoError(t, os.MkdirAll(dir, 0o750))          //nolint:gosec // a path the operator chose
	f, err := os.Create(filepath.Join(dir, "cpu.pprof")) //nolint:gosec // a path the operator chose
	require.NoError(t, err)
	require.NoError(t, pprof.StartCPUProfile(f))

	return func() {
		pprof.StopCPUProfile()
		require.NoError(t, f.Close())
	}
}

// writeHeapProfile writes heap.pprof to $REPORTER_PERF_OUT, when set.
func writeHeapProfile(t *testing.T) {
	t.Helper()

	dir := os.Getenv("REPORTER_PERF_OUT")
	if dir == "" {
		return
	}

	f, err := os.Create(filepath.Join(dir, "heap.pprof")) //nolint:gosec // a path the operator chose
	require.NoError(t, err)
	require.NoError(t, pprof.WriteHeapProfile(f))
	require.NoError(t, f.Close())
}
