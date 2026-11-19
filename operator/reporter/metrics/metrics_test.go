package metrics_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/metrics"
)

const labelResult = "result"

func TestMetricsNilSafe(t *testing.T) {
	var m *metrics.Metrics

	require.NotPanics(t, func() {
		m.ObservePass(time.Second, 10)
		m.RecordReview("ok")
		m.RecordAlert("light-client-changed")
		m.RecordRPC("akash", "status", time.Millisecond, errors.New("boom"))
		m.RecordRPC("solana", "getSlot", time.Millisecond, nil)
		m.RecordCache("snapshot", true)
		m.RecordTx("included")
		m.RecordTxMessage("MsgReportOutcome")
		m.AddTxFees(1000)
		m.AddTxInFlight(1)
		m.AddTxInFlight(-1)
		m.RecordDeferred("slot_busy")
	})
}

func TestMetricsRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	m.ObservePass(2*time.Second, 7)
	m.RecordReview("ok")
	m.RecordReview("error")
	m.RecordAlert("audit-unavailable")
	m.RecordRPC("akash", "status", time.Millisecond, nil)
	m.RecordRPC("solana", "getSlot", time.Millisecond, errors.New("boom"))
	m.RecordCache("snapshot", true)
	m.RecordCache("snapshot", false)
	m.RecordTx("included")
	m.RecordTxMessage("/akash.sink.v1.MsgReportOutcome")
	m.AddTxFees(5000)
	m.AddTxInFlight(2)
	m.RecordDeferred("cutoff")

	want := map[string][]string{
		"akt_reporter_pass_duration_seconds":   nil,
		"akt_reporter_pass_transfers":          nil,
		"akt_reporter_reviews_total":           {labelResult},
		"akt_reporter_alerts_total":            {"alert"},
		"akt_reporter_rpc_duration_seconds":    {"chain", "method"},
		"akt_reporter_rpc_errors_total":        {"chain", "method"},
		"akt_reporter_cache_requests_total":    {"cache", labelResult},
		"akt_reporter_tx_total":                {labelResult},
		"akt_reporter_tx_messages_total":       {"type"},
		"akt_reporter_tx_fees_uakt_total":      nil,
		"akt_reporter_tx_in_flight":            nil,
		"akt_reporter_deferred_messages_total": {"reason"},
	}

	families, err := reg.Gather()
	require.NoError(t, err)

	got := make(map[string][]string, len(families))

	for _, family := range families {
		require.NotEmpty(t, family.GetMetric(), family.GetName())

		var labels []string

		for _, pair := range family.GetMetric()[0].GetLabel() {
			labels = append(labels, pair.GetName())
		}

		slices.Sort(labels)
		got[family.GetName()] = labels
	}

	require.Equal(t, want, got)
}

func TestMetricsValues(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	m.RecordRPC("akash", "status", time.Millisecond, errors.New("boom"))
	m.RecordCache("snapshot", true)
	m.RecordCache("snapshot", false)
	m.RecordCache("snapshot", false)
	m.AddTxInFlight(3)
	m.AddTxInFlight(-1)
	m.ObservePass(time.Second, 42)

	families, err := reg.Gather()
	require.NoError(t, err)

	values := map[string]float64{}

	for _, family := range families {
		for _, metric := range family.GetMetric() {
			key := family.GetName()
			for _, pair := range metric.GetLabel() {
				key += "|" + pair.GetValue()
			}

			switch {
			case metric.GetCounter() != nil:
				values[key] = metric.GetCounter().GetValue()
			case metric.GetGauge() != nil:
				values[key] = metric.GetGauge().GetValue()
			case metric.GetHistogram() != nil:
				values[key] = float64(metric.GetHistogram().GetSampleCount())
			}
		}
	}

	require.InEpsilon(t, 1, values["akt_reporter_rpc_errors_total|akash|status"], 0)
	require.InEpsilon(t, 1, values["akt_reporter_rpc_duration_seconds|akash|status"], 0)
	require.InEpsilon(t, 1, values["akt_reporter_cache_requests_total|snapshot|hit"], 0)
	require.InEpsilon(t, 2, values["akt_reporter_cache_requests_total|snapshot|miss"], 0)
	require.InEpsilon(t, 2, values["akt_reporter_tx_in_flight"], 0)
	require.InEpsilon(t, 42, values["akt_reporter_pass_transfers"], 0)
	require.InEpsilon(t, 1, values["akt_reporter_pass_duration_seconds"], 0)
}

func TestServe(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	m.RecordReview("ok")

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := lis.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)

	go func() { done <- metrics.Serve(ctx, lis, reg) }()

	var body []byte

	require.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr)) //nolint:noctx
		if err != nil {
			return false
		}

		defer func() { _ = resp.Body.Close() }()

		body, err = io.ReadAll(resp.Body)

		return err == nil && resp.StatusCode == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)

	require.Contains(t, string(body), `akt_reporter_reviews_total{result="ok"} 1`)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}

func TestServeAcceptError(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	err = metrics.Serve(context.Background(), lis, prometheus.NewRegistry())
	require.ErrorIs(t, err, net.ErrClosed)
}
