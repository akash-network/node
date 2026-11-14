package tx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
)

// The drain of spec §11.3: drain-5000's votes against a node that commits a block every 6 s of chain time, with the
// window and the block interval scaled down by the same factor.
const (
	drainVotes   = 5_000
	drainScale   = 1.0 / 20
	drainRefresh = 50 * time.Millisecond
	// drainWindows is how many windows after the starting one the drain may take.
	drainWindows = 8
	signerDown   = "signer unavailable"
)

// flakySigner takes delay per signature. From its after-th call on, it fails every call for outage of real time.
type flakySigner struct {
	signer.Signer
	delay  time.Duration
	after  int64
	outage time.Duration

	calls  atomic.Int64
	failed atomic.Int64
	mu     sync.Mutex
	until  time.Time
}

func (s *flakySigner) Sign(ctx context.Context, signBytes []byte) ([]byte, error) {
	n := s.calls.Add(1)

	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	s.mu.Lock()
	if n == s.after {
		s.until = time.Now().Add(s.outage)
	}
	down := time.Now().Before(s.until)
	s.mu.Unlock()

	if down {
		s.failed.Add(1)
		return nil, errors.New(signerDown)
	}

	return s.Signer.Sign(ctx, signBytes)
}

func storeMax(peak *atomic.Int64, n int) {
	for {
		old := peak.Load()
		if int64(n) <= old || peak.CompareAndSwap(old, int64(n)) {
			return
		}
	}
}

// drain runs drain-5000 to the end through one broadcaster whose signer wrap returns, starting 90% into a window,
// with passes as start runs them. It checks the drain's bounds and returns the error events written.
func drain(t *testing.T, wrap func(signer.Signer) signer.Signer) map[string]string {
	t.Helper()

	scaleTiming(t, drainScale)
	interval := timing.window / 20
	h := newHarness(t, setup{real: true, scenario: "drain-5000", blockInterval: interval, minRefresh: drainRefresh})
	require.Len(t, h.ids, drainVotes)

	// Unconfirmed transactions and the in-flight count are sampled after every change of the owner's state.
	var unconfirmed, inFlight atomic.Int64
	o := h.realOptions()
	o.RefreshInterval = drainRefresh
	o.changed = func(s *ownerState) {
		n := 0
		for _, e := range s.entries {
			if tx := e.tx; tx != nil && tx.phase == sent && tx.observed {
				n++
			}
		}
		storeMax(&unconfirmed, n)
		storeMax(&inFlight, s.inFlight)
	}
	p := h.start(o, wrap(h.signer), h.dial())
	s := p.wire(nil)

	// The first pass plans from a block 90% into window w0.
	ninety := timing.window * 9 / 10
	w0 := WindowOf(time.Now().Add(h.lead))
	if time.Now().Add(h.lead).After(WindowStart(w0).Add(ninety)) {
		w0++
	}
	time.Sleep(time.Until(WindowStart(w0).Add(ninety + interval).Add(-h.lead)))
	into := h.snapshot().Time.Sub(WindowStart(w0))
	require.GreaterOrEqual(t, into, ninety-interval/2)
	require.Less(t, into, timing.window)

	// Passes run as start runs them, scan.interval apart: 15 s, an eighth of the window.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	firstPass := make(chan time.Time, 1)
	loopDone := make(chan struct{})
	passes := 0
	began := time.Now()
	go func() {
		defer close(loopDone)
		for ctx.Err() == nil {
			s.Pass(ctx, nil)
			p.b.Flush()
			passes++
			select {
			case firstPass <- time.Now():
			default:
			}

			select {
			case <-ctx.Done():
			case <-time.After(timing.window / 8):
			}
		}
	}()

	deadline := WindowStart(w0 + drainWindows + 1).Add(-h.lead)
	var ended time.Time
	for {
		ex := h.node.Executions()
		done := 0
		for _, id := range h.ids {
			if ex[executedVote(id)] > 0 {
				done++
			}
		}
		if done == drainVotes {
			ended = time.Now()
			break
		}
		require.True(t, time.Now().Before(deadline), "%d of %d votes executed after %d windows", done, drainVotes, drainWindows)
		time.Sleep(50 * time.Millisecond)
	}
	used := WindowOf(h.snapshot().Time) - w0

	cancel()
	<-loopDone
	first := <-firstPass

	t.Logf("started %.0f%% into window %d; drained in %d windows (%s) over %d passes; first pass took %s; peak %d unconfirmed, %d in flight",
		100*into.Seconds()/timing.window.Seconds(), w0, used, ended.Sub(began).Round(time.Millisecond), passes,
		first.Sub(began).Round(time.Millisecond), unconfirmed.Load(), inFlight.Load())

	require.LessOrEqual(t, used, int64(drainWindows))
	require.True(t, first.Before(ended), "the first pass returns before the drain ends")
	require.LessOrEqual(t, unconfirmed.Load(), int64(o.MaxInFlight))
	require.LessOrEqual(t, inFlight.Load(), int64(o.MaxInFlight))

	ex := h.executions()
	for _, id := range h.ids {
		require.Equal(t, 1, ex[executedVote(id)], id.String())
	}

	// Every vote is confirmed to the reporter once its lookups have run.
	require.Eventually(t, func() bool {
		idle := false
		p.state(func(s *ownerState) { idle = s.inFlight == 0 })
		return idle
	}, 10*time.Second, 10*time.Millisecond)
	hashes, errs := outcomes(t, p.lines())
	for _, id := range h.ids {
		require.Contains(t, hashes, id.String())
	}

	return errs
}

func TestDrain(t *testing.T) {
	t.Run("fast signer", func(t *testing.T) {
		require.Empty(t, drain(t, func(s signer.Signer) signer.Signer { return s }))
	})

	// The spec's 100 ms per signature and 10 s outage, scaled down.
	t.Run("signer recovery", func(t *testing.T) {
		var flaky *flakySigner
		errs := drain(t, func(s signer.Signer) signer.Signer {
			flaky = &flakySigner{Signer: s, delay: 5 * time.Millisecond, after: 20, outage: 500 * time.Millisecond}
			return flaky
		})

		require.Positive(t, flaky.failed.Load())
		require.NotEmpty(t, errs, "the messages that failed to sign are released with an error")
		for id, e := range errs {
			require.Contains(t, e, signerDown, id)
		}
	})
}
