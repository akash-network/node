package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/config"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/metrics"
	"pkg.akt.dev/node/v2/operator/reporter/observe"
	"pkg.akt.dev/node/v2/operator/reporter/scan"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	// evidenceSlots and evidenceBytes bound the evidence index cache (spec §5.6).
	evidenceSlots = 256
	evidenceBytes = 64 << 20

	modePreview = "preview"
)

var errBroadcastUnavailable = errors.New("broadcast mode is not available yet; set broadcast = false to preview")

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Run a pass, wait scan.interval, and repeat until stopped",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReporter(cmd, false)
		},
	}
}

func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Run one pass; exit 1 if anything failed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReporter(cmd, true)
		},
	}
}

// reporter is the wiring of one start or scan run.
type reporter struct {
	cfg     config.Resolved
	log     *zerolog.Logger
	akash   *akash.Client
	events  *events.Writer
	scanner *scan.Scanner
	metrics *metrics.Metrics
	gather  prometheus.Gatherer
}

// runReporter validates the configuration, connects, starts the metrics listener and runs one pass (scan) or passes
// until the command's context ends (start).
func runReporter(cmd *cobra.Command, once bool) (err error) {
	ctx := cmd.Context()

	cfg, err := resolveConfig(cmd)
	if err != nil {
		return err
	}
	if cfg.Broadcast {
		return errBroadcastUnavailable
	}

	r := &reporter{cfg: cfg, log: zerolog.Ctx(ctx)}
	logStartup(r.log, cfg)

	if err := r.connect(ctx, cmd); err != nil {
		// A signal during startup stops start like one between passes (spec §5.7).
		if !once && ctx.Err() != nil {
			r.log.Info().Err(err).Msg("stopped during startup")
			return nil
		}

		return err
	}
	defer func() {
		if cerr := r.close(); err == nil {
			err = cerr
		}
	}()

	// A failing metrics listener stops the run.
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	stopMetrics := r.serveMetrics(runCtx, cancel)
	defer stopMetrics()

	if once {
		failures := r.scanner.Pass(runCtx, nil)
		if cause := listenerFailure(ctx, runCtx); cause != nil {
			return cause
		}
		if failures > 0 {
			return exitCode(1)
		}

		return nil
	}

	r.loop(runCtx, cfg.Scan.Interval)

	// A signal ends start with exit code 0 (spec §5.7).
	return listenerFailure(ctx, runCtx)
}

// connect dials Akash, which checks that both endpoints are one node, and builds the Solana client, the observer and
// the scanner.
func (r *reporter) connect(ctx context.Context, cmd *cobra.Command) error {
	cfg := r.cfg

	reg := prometheus.NewRegistry()
	r.metrics = metrics.New(reg)
	r.gather = reg

	a, err := akash.Dial(ctx, akash.Options{
		Node:           cfg.Node,
		GRPCAddr:       cfg.GRPCAddr,
		GRPCInsecure:   cfg.GRPCInsecure,
		ChainID:        cfg.ChainID,
		MaxAge:         cfg.MaxAge,
		RequestTimeout: cfg.RequestTimeout,
		Concurrency:    cfg.Scan.AkashConcurrency,
		Metrics:        r.metrics,
	})
	if err != nil {
		return fmt.Errorf("connect to Akash: %w", err)
	}
	r.akash = a
	r.log.Info().Str("node", cfg.Node).Str("grpc-addr", cfg.GRPCAddr).Bool("tx-index", a.TxIndexEnabled()).
		Msg("connected to Akash")

	rpc := aktsolana.NewRPC(aktsolana.RPCOptions{
		URL:              cfg.Solana.RPC,
		Timeout:          cfg.RequestTimeout,
		Concurrency:      cfg.Scan.SolanaConcurrency,
		BlockConcurrency: cfg.Scan.BlockConcurrency,
		Metrics:          r.metrics,
	})
	index := aktsolana.NewEvidenceIndex(rpc, cfg.ProgramID, evidenceSlots, evidenceBytes)

	r.events = events.NewWriter(cmd.OutOrStdout())

	// The observer reports light-client changes to the scanner, which is built after it. OnChange writes synchronously.
	var sc *scan.Scanner
	observer := observe.New(rpc, index, observe.Options{
		GenesisHash:    cfg.Solana.GenesisHash,
		ProgramID:      cfg.ProgramID,
		Mint:           cfg.Mint,
		LightClientPin: cfg.LightClientPin,
		ChainID:        cfg.ChainID,
		MaxAge:         cfg.MaxAge,
		SlotLead:       cfg.SlotLead,
	}, func(c observe.Change) { sc.OnChange(c) })

	sc = scan.New(a, observer, r.events, scan.Options{
		Reporter:     cfg.Config.Reporter,
		Broadcast:    false,
		Workers:      cfg.Scan.Workers,
		PageSize:     cfg.Scan.PageSize,
		AuditPerScan: cfg.Scan.AuditPerScan,
		Metrics:      r.metrics,
	}, nil)
	r.scanner = sc

	return nil
}

// serveMetrics starts the metrics listener when metrics.listen is set. A listener error cancels ctx with its cause.
// The returned function stops the listener and waits for it.
func (r *reporter) serveMetrics(ctx context.Context, fail context.CancelCauseFunc) func() {
	addr := r.cfg.Metrics.Listen
	if addr == "" {
		return func() {}
	}

	ctx, stop := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := metrics.Serve(ctx, addr, r.gather); err != nil {
			fail(&listenerError{err: err})
		}
	}()

	r.log.Info().Str("listen", addr).Msg("serving metrics")

	return func() {
		stop()
		wg.Wait()
	}
}

// loop runs passes until ctx ends, waiting interval between them. It never begins a pass once ctx is done.
func (r *reporter) loop(ctx context.Context, interval time.Duration) {
	for ctx.Err() == nil {
		r.scanner.Pass(ctx, nil)

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}

	r.log.Info().Msg("stopped")
}

// close flushes the events and releases the Akash connections.
func (r *reporter) close() error {
	werr := r.events.Close()
	aerr := r.akash.Close()

	return errors.Join(werr, aerr)
}

type listenerError struct{ err error }

func (e *listenerError) Error() string { return "metrics listener: " + e.err.Error() }
func (e *listenerError) Unwrap() error { return e.err }

// listenerFailure returns the metrics listener error that ended runCtx, if that is what ended it.
func listenerFailure(parent, runCtx context.Context) error {
	if parent.Err() != nil {
		return nil
	}

	var lerr *listenerError
	if errors.As(context.Cause(runCtx), &lerr) {
		return lerr
	}

	return nil
}

// logStartup logs the effective configuration without key material (spec §9.2, §13).
func logStartup(log *zerolog.Logger, cfg config.Resolved) {
	lightClient := "follow"
	if cfg.LightClientPin != nil {
		lightClient = cfg.LightClientPin.String()
	}

	log.Info().
		Str("mode", modePreview).
		Str("network", cfg.Network).
		Str("chain-id", cfg.ChainID).
		Str("reporter", cfg.Config.Reporter).
		Str("node", cfg.Node).
		Str("grpc-addr", cfg.GRPCAddr).
		Str("solana.rpc", cfg.Solana.RPC).
		Str("solana.program-id", cfg.Solana.ProgramID).
		Str("solana.light-client-program", lightClient).
		Stringer("max-age", cfg.MaxAge).
		Stringer("scan.interval", cfg.Scan.Interval).
		Int("scan.workers", cfg.Scan.Workers).
		Str("metrics.listen", cfg.Metrics.Listen).
		Msg("starting reporter")
}
