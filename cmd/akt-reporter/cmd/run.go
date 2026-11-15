package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/config"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/metrics"
	"pkg.akt.dev/node/v2/operator/reporter/observe"
	"pkg.akt.dev/node/v2/operator/reporter/scan"
	"pkg.akt.dev/node/v2/operator/reporter/signer"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
	"pkg.akt.dev/node/v2/operator/reporter/tx"
)

const (
	// evidenceSlots and evidenceBytes bound the evidence index cache (spec §5.6).
	evidenceSlots = 256
	evidenceBytes = 64 << 20

	// shutdownGrace is how long the broadcaster waits for sent transactions after a signal (spec §5.7).
	shutdownGrace = 30 * time.Second
	// settleMargin keeps scan's settle waiting past the broadcaster's grace, so that the broadcaster ends the wait
	// and no outcome is written after the summary.
	settleMargin = 5 * time.Second

	modePreview   = "preview"
	modeBroadcast = "broadcast"
)

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

// reporter is the wiring of one start or scan run. signer and broadcaster are nil in preview.
type reporter struct {
	cfg         config.Resolved
	log         *zerolog.Logger
	akash       *akash.Client
	signer      signer.Signer
	broadcaster *tx.Broadcaster
	events      *events.Writer
	scanner     *scan.Scanner
	metrics     *metrics.Metrics
	gather      prometheus.Gatherer
	// blockMaxGas is the chain's consensus block max gas, read at broadcast startup.
	blockMaxGas int64
}

// runReporter validates the configuration, connects, starts the metrics listener and runs one pass (scan) or passes
// until the command's context ends (start). In broadcast mode the broadcaster runs across the passes.
func runReporter(cmd *cobra.Command, once bool) (err error) {
	ctx := cmd.Context()

	cfg, err := resolveConfig(cmd)
	if err != nil {
		return err
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

	// A signal or a failing metrics listener ends runCtx: no new pass, review or signing starts.
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	stopMetrics := r.serveMetrics(runCtx, cancel)
	defer stopMetrics()

	// Deferred last, so it runs first: the broadcaster stops before the signer, the clients and the events close.
	stopBroadcaster := r.startBroadcaster(runCtx)
	defer stopBroadcaster()

	if once {
		failures := r.scanOnce(runCtx)
		if cause := listenerFailure(ctx, runCtx); cause != nil {
			return cause
		}
		if failures > 0 {
			return exitCode(1)
		}

		return nil
	}

	r.loop(runCtx, cfg.Scan.Interval, stopBroadcaster)

	// A signal ends start with exit code 0 (spec §5.7).
	return listenerFailure(ctx, runCtx)
}

// connect dials Akash, which checks that both endpoints are one node, runs the broadcast startup checks, and builds
// the Solana client, the observer, the scanner and, in broadcast mode, the broadcaster. On error it releases what it
// opened.
func (r *reporter) connect(ctx context.Context, cmd *cobra.Command) (err error) {
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
	defer func() {
		if err != nil {
			_ = r.release()
		}
	}()

	r.log.Info().Str("node", cfg.Node).Str("grpc-addr", cfg.GRPCAddr).Bool("tx-index", a.TxIndexEnabled()).
		Msg("connected to Akash")

	var accountNumber uint64
	if cfg.Broadcast {
		if accountNumber, err = r.prepareBroadcast(ctx, cmd); err != nil {
			return err
		}
	}

	rpc := aktsolana.NewRPC(aktsolana.RPCOptions{
		URL:              cfg.Solana.RPC,
		Timeout:          cfg.RequestTimeout,
		Concurrency:      cfg.Scan.SolanaConcurrency,
		BlockConcurrency: cfg.Scan.BlockConcurrency,
		Metrics:          r.metrics,
	})
	index := aktsolana.NewEvidenceIndex(rpc, cfg.ProgramID, evidenceSlots, evidenceBytes)

	r.events = events.NewWriter(cmd.OutOrStdout())

	// The scanner hands its plans to the broadcaster, which runs its follow-up reviews through the scanner.
	var queuer scan.Queuer
	if cfg.Broadcast {
		r.broadcaster = r.newBroadcaster(accountNumber)
		queuer = r.broadcaster
	}

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
		Broadcast:    cfg.Broadcast,
		Workers:      cfg.Scan.Workers,
		PageSize:     cfg.Scan.PageSize,
		AuditPerScan: cfg.Scan.AuditPerScan,
		Metrics:      r.metrics,
	}, queuer)
	r.scanner = sc

	if r.broadcaster != nil {
		r.broadcaster.SetReviewer(sc)
	}

	return nil
}

// prepareBroadcast runs the broadcast startup checks that follow the node-ID check, in order: the first snapshot,
// the signer, which must derive reporter, and the reporter's account number, which it returns. It then reads the
// block max gas, which caps the gas limit below tx.max-gas when it is lower, and logs the cap. Run logs the
// transaction-index warning. Preview never calls it, so it never opens the keyring or dials the remote signer
// (spec §7).
func (r *reporter) prepareBroadcast(ctx context.Context, cmd *cobra.Command) (uint64, error) {
	cfg := r.cfg

	if _, err := r.akash.Latest(ctx); err != nil {
		return 0, fmt.Errorf("read Akash state: %w", err)
	}

	kind := "keyring"
	if cfg.From != "" {
		kr, err := newKeyring(cfg.KeyringBackend, cfg.KeyringDir, cmd.InOrStdin(), sdkutil.MakeEncodingConfig().Codec)
		if err != nil {
			return 0, err
		}

		if r.signer, err = signer.NewKeyring(kr, cfg.From, cfg.Reporter); err != nil {
			return 0, fmt.Errorf("keyring signer %q: %w", cfg.From, err)
		}
	} else {
		kind = "remote"
		remote := cfg.Signer.Remote

		s, err := signer.NewRemote(ctx, signer.RemoteOptions{
			Address:    remote.Address,
			KeyID:      remote.KeyID,
			CAFile:     remote.CAFile,
			ServerName: remote.ServerName,
			CertFile:   remote.CertFile,
			KeyFile:    remote.KeyFile,
			Timeout:    remote.Timeout,
		}, cfg.Reporter)
		if err != nil {
			return 0, fmt.Errorf("remote signer %s: %w", remote.Address, err)
		}
		r.signer = s
	}

	accountNumber, err := r.akash.AccountNumber(ctx, cfg.Reporter)
	if err != nil {
		return 0, fmt.Errorf("read the account of reporter %s: %w", cfg.Config.Reporter, err)
	}

	r.log.Info().Str("signer", kind).Str("address", r.signer.Address().String()).Uint64("account-number", accountNumber).
		Msg("loaded signer")

	if r.blockMaxGas, err = r.akash.BlockMaxGas(ctx); err != nil {
		return 0, fmt.Errorf("read the consensus block max gas: %w", err)
	}

	r.log.Info().Uint64("tx.max-gas", cfg.Tx.MaxGas).Int64("block-max-gas", r.blockMaxGas).
		Uint64("gas-cap", tx.EffectiveMaxGas(cfg.Tx.MaxGas, r.blockMaxGas)).Msg("gas cap")

	return accountNumber, nil
}

// signerTimeout is the remote signer's timeout, which lengthens the send cutoff, or 0 for the keyring.
func (r *reporter) signerTimeout() time.Duration {
	if r.cfg.From != "" {
		return 0
	}

	return r.cfg.Signer.Remote.Timeout
}

func (r *reporter) newBroadcaster(accountNumber uint64) *tx.Broadcaster {
	cfg := r.cfg

	return tx.New(tx.Options{
		Reporter:      cfg.Config.Reporter,
		MaxMsgs:       cfg.Tx.MaxMsgs,
		MaxInFlight:   cfg.Tx.MaxInFlight,
		GasPerMessage: cfg.GasPerMessage,
		GasAdjustment: cfg.GasAdjustment,
		GasPrices:     cfg.GasPrices,
		Fees:          cfg.Fees,
		MaxGas:        cfg.Tx.MaxGas,
		BlockMaxGas:   r.blockMaxGas,
		MaxFee:        cfg.MaxFee,
		SignerTimeout: r.signerTimeout(),
		ShutdownGrace: shutdownGrace,
		Metrics:       r.metrics,
	}, r.akash, tx.NewBuilder(txConfig(), cfg.ChainID, accountNumber, r.signer), r.events)
}

// txConfig encodes the reporter's transactions with akash's encoding config and the sink types.
func txConfig() client.TxConfig {
	enc := sdkutil.MakeEncodingConfig()
	sinkv1.RegisterInterfaces(enc.InterfaceRegistry)

	return enc.TxConfig
}

// startBroadcaster runs the broadcaster, if any, until ctx ends; ctx carries the logger that Run warns through. The
// returned function stops it and waits until it returns: once its context ends nothing new is signed, and sent
// transactions get the broadcaster's shutdown grace. It may be called more than once.
func (r *reporter) startBroadcaster(ctx context.Context) func() {
	b := r.broadcaster
	if b == nil {
		return func() {}
	}

	ctx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)

		if err := b.Run(ctx); err != nil {
			r.log.Error().Err(err).Msg("broadcaster stopped")
		}
	}()

	return sync.OnceFunc(func() {
		stop()
		<-done
	})
}

// scanOnce runs scan's pass. In broadcast mode its settle waits for the pass's transactions and follow-up reviews.
// Once a signal ends ctx, the broadcaster's shutdown grace ends the wait when it returns; settleCtx outlasts that
// grace and only bounds a broadcaster that never returns (spec §5.7).
func (r *reporter) scanOnce(ctx context.Context) int {
	b := r.broadcaster
	if b == nil {
		return r.scanner.Pass(ctx, nil)
	}

	settleCtx, cancel := graceContext(ctx, shutdownGrace+settleMargin)
	defer cancel()

	return r.scanner.Pass(ctx, func() int { b.Flush(); return b.Wait(settleCtx) })
}

// graceContext returns a context that ends grace after ctx does, or when cancel is called.
func graceContext(ctx context.Context, grace time.Duration) (context.Context, context.CancelFunc) {
	gctx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	stop := context.AfterFunc(ctx, func() {
		t := time.NewTimer(grace)
		defer t.Stop()

		select {
		case <-t.C:
			cancel()
		case <-gctx.Done():
		}
	})

	return gctx, func() {
		stop()
		cancel()
	}
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

// loop runs passes until ctx ends, waiting interval between them. It never begins a pass once ctx is done. In
// broadcast mode a pass counts the transaction failures resolved since the previous pass; it does not wait for its
// own transactions, which the next pass counts. A pass that a signal ended has no next pass: its settle stops the
// broadcaster, which gives sent transactions their grace, so that their outcomes precede and count in its summary
// (spec §5.7). After a signal between passes, the caller's stopBroadcaster gives that grace instead, and outcome
// events may follow the last summary (spec §9.1).
func (r *reporter) loop(ctx context.Context, interval time.Duration, stopBroadcaster func()) {
	var settle func() int
	if b := r.broadcaster; b != nil {
		settle = func() int {
			b.Flush()
			if ctx.Err() != nil {
				stopBroadcaster()
			}
			return b.TakeFailures()
		}
	}

	for ctx.Err() == nil {
		r.scanner.Pass(ctx, settle)

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}

	r.log.Info().Msg("stopped")
}

// close releases the signer and the Akash connections, then flushes the events last, so that the pass's summary or
// error stays the last line (spec §5.7). The broadcaster must have stopped.
func (r *reporter) close() error {
	rerr := r.release()
	werr := r.events.Close()

	return errors.Join(rerr, werr)
}

// release closes the remote signer, if any, and the Akash client.
func (r *reporter) release() error {
	var serr error
	if c, ok := r.signer.(io.Closer); ok {
		serr = c.Close()
	}

	return errors.Join(serr, r.akash.Close())
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

	mode := modePreview
	if cfg.Broadcast {
		mode = modeBroadcast
	}

	log.Info().
		Str("mode", mode).
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
