// Package metrics defines the reporter's Prometheus collectors and serves them.
package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace       = "akt_reporter"
	shutdownTimeout = 5 * time.Second
	headerTimeout   = 10 * time.Second

	labelResult = "result"
)

// Metrics holds the collectors of spec section 9.3. Every method is safe on a nil *Metrics and records nothing.
type Metrics struct {
	PassDuration     prometheus.Histogram
	PassTransfers    prometheus.Gauge
	Reviews          *prometheus.CounterVec
	Alerts           *prometheus.CounterVec
	RPCDuration      *prometheus.HistogramVec
	RPCErrors        *prometheus.CounterVec
	CacheRequests    *prometheus.CounterVec
	Tx               *prometheus.CounterVec
	TxMessages       *prometheus.CounterVec
	TxFeesUakt       prometheus.Counter
	TxInFlight       prometheus.Gauge
	DeferredMessages *prometheus.CounterVec
}

// New creates the collectors and registers them with reg.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		PassDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "pass_duration_seconds",
			Help:      "Duration of a reporter pass.",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 14),
		}),
		PassTransfers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pass_transfers",
			Help:      "Transfers reviewed in the last pass.",
		}),
		Reviews: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reviews_total",
			Help:      "Transfer reviews by result.",
		}, []string{labelResult}),
		Alerts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "alerts_total",
			Help:      "Alerts raised by kind.",
		}, []string{"alert"}),
		RPCDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "rpc_duration_seconds",
			Help:      "Duration of RPC calls.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"chain", "method"}),
		RPCErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rpc_errors_total",
			Help:      "Failed RPC calls.",
		}, []string{"chain", "method"}),
		CacheRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_requests_total",
			Help:      "Cache lookups by cache and result.",
		}, []string{"cache", labelResult}),
		Tx: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tx_total",
			Help:      "Sink transactions by result.",
		}, []string{labelResult}),
		TxMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tx_messages_total",
			Help:      "Sink messages sent by type.",
		}, []string{"type"}),
		TxFeesUakt: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tx_fees_uakt_total",
			Help:      "Fees paid for sink transactions, in uakt.",
		}),
		TxInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "tx_in_flight",
			Help:      "Sink transactions awaiting confirmation.",
		}),
		DeferredMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "deferred_messages_total",
			Help:      "Sink messages deferred by reason.",
		}, []string{"reason"}),
	}

	reg.MustRegister(
		m.PassDuration,
		m.PassTransfers,
		m.Reviews,
		m.Alerts,
		m.RPCDuration,
		m.RPCErrors,
		m.CacheRequests,
		m.Tx,
		m.TxMessages,
		m.TxFeesUakt,
		m.TxInFlight,
		m.DeferredMessages,
	)

	return m
}

// ObservePass records a finished pass and the number of transfers it reviewed.
func (m *Metrics) ObservePass(d time.Duration, transfers int) {
	if m == nil {
		return
	}

	m.PassDuration.Observe(d.Seconds())
	m.PassTransfers.Set(float64(transfers))
}

// RecordReview counts a review with result "ok" or "error".
func (m *Metrics) RecordReview(result string) {
	if m == nil {
		return
	}

	m.Reviews.WithLabelValues(result).Inc()
}

// RecordAlert counts an alert of the given kind.
func (m *Metrics) RecordAlert(alert string) {
	if m == nil {
		return
	}

	m.Alerts.WithLabelValues(alert).Inc()
}

// RecordRPC records an RPC call on chain "akash" or "solana"; a non-nil err also counts as an error.
func (m *Metrics) RecordRPC(chain, method string, d time.Duration, err error) {
	if m == nil {
		return
	}

	m.RPCDuration.WithLabelValues(chain, method).Observe(d.Seconds())

	if err != nil {
		m.RPCErrors.WithLabelValues(chain, method).Inc()
	}
}

// RecordCache counts a lookup in the named cache as a hit or a miss.
func (m *Metrics) RecordCache(cache string, hit bool) {
	if m == nil {
		return
	}

	result := "miss"
	if hit {
		result = "hit"
	}

	m.CacheRequests.WithLabelValues(cache, result).Inc()
}

// RecordTx counts a transaction with result included, failed, rejected, slot_taken, expired or unconfirmed.
func (m *Metrics) RecordTx(result string) {
	if m == nil {
		return
	}

	m.Tx.WithLabelValues(result).Inc()
}

// RecordTxMessage counts a message sent in a transaction.
func (m *Metrics) RecordTxMessage(msgType string) {
	if m == nil {
		return
	}

	m.TxMessages.WithLabelValues(msgType).Inc()
}

// AddTxFees adds fees paid, in uakt.
func (m *Metrics) AddTxFees(uakt float64) {
	if m == nil {
		return
	}

	m.TxFeesUakt.Add(uakt)
}

// AddTxInFlight changes the number of transactions awaiting confirmation by delta.
func (m *Metrics) AddTxInFlight(delta float64) {
	if m == nil {
		return
	}

	m.TxInFlight.Add(delta)
}

// RecordDeferred counts a deferred message with reason slot_busy, slot_full, cutoff, simulation or cap.
func (m *Metrics) RecordDeferred(reason string) {
	if m == nil {
		return
	}

	m.DeferredMessages.WithLabelValues(reason).Inc()
}

// Serve exposes g at /metrics on addr until ctx is cancelled, then shuts down gracefully.
func Serve(ctx context.Context, addr string, g prometheus.Gatherer) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(g, promhttp.HandlerOpts{}))

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: headerTimeout,
	}

	lc := net.ListenConfig{}

	lis, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	errc := make(chan error, 1)

	go func() { errc <- srv.Serve(lis) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
