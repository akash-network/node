package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestLogEndpoint(t *testing.T) {
	for raw, want := range map[string]string{
		"http://127.0.0.1:26657/SECRET?token=SECRET": "http://127.0.0.1:26657",
		"tcp://localhost:26657":                      "tcp://localhost:26657",
		"https://user:SECRET@rpc.example.com/SECRET": "https://rpc.example.com",
		"http://[::1]:26657/SECRET":                  "http://[::1]:26657",
		// Unparsable, or no host: the raw string is never logged.
		"http://SECRET:%Q/":  "invalid URL",
		"http://[::1/SECRET": "invalid URL",
		"http:///SECRET":     "invalid URL",
		"localhost:SECRET":   "invalid URL",
		"":                   "invalid URL",
	} {
		require.Equal(t, want, logEndpoint(raw), raw)
	}
}

// metricsReporter returns a reporter with metrics.listen set to listen and an empty registry. It logs JSON to logs.
func metricsReporter(listen string, logs *bytes.Buffer) *reporter {
	logger := zerolog.New(logs)
	r := &reporter{log: &logger, gather: prometheus.NewRegistry()}
	r.cfg.Metrics.Listen = listen

	return r
}

func TestServeMetricsBindsBeforeReturning(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "localhost:0"} {
		t.Run(addr, func(t *testing.T) {
			host, _, err := net.SplitHostPort(addr)
			require.NoError(t, err)
			if _, err := net.DefaultResolver.LookupHost(t.Context(), host); err != nil {
				t.Skipf("%s does not resolve: %v", host, err)
			}

			var logs bytes.Buffer
			r := metricsReporter(addr, &logs)
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)

			stop, err := r.serveMetrics(ctx, cancel)
			require.NoError(t, err)

			var line struct{ Listen, Message string }
			require.NoError(t, json.Unmarshal(logs.Bytes(), &line))
			require.Equal(t, "serving metrics", line.Message)
			_, port, err := net.SplitHostPort(line.Listen)
			require.NoError(t, err)
			require.NotEqual(t, "0", port, "the log names the bound port")

			// No retry: runReporter starts the pass as soon as serveMetrics returns.
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+line.Listen+"/metrics", nil)
			require.NoError(t, err)
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.NoError(t, res.Body.Close())
			require.Equal(t, http.StatusOK, res.StatusCode)

			stop()
			require.NoError(t, context.Cause(ctx), "the listener did not fail the run")
		})
	}
}

func TestServeMetricsBindFailure(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })

	var logs bytes.Buffer
	r := metricsReporter(taken.Addr().String(), &logs)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)

	stop, err := r.serveMetrics(ctx, cancel)
	var lerr *listenerError
	require.ErrorAs(t, err, &lerr)
	require.ErrorContains(t, err, "metrics listener: listen tcp "+taken.Addr().String())
	require.NoError(t, context.Cause(ctx), "a bind error is returned, not sent to fail")
	require.Empty(t, logs.String(), "nothing is served")

	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop blocked after a failed bind")
	}
}
