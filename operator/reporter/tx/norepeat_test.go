package tx

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// These tests plan only through real scanners over the node, never with synthetic plans (spec §11.3). They run on the
// system clock with the nonce scheme's durations scaled down.

// producer commits a block every interval of real time while not paused.
type producer struct {
	paused atomic.Bool
}

func (h *harness) produce(every time.Duration) *producer {
	p := &producer{}
	stop := make(chan struct{})

	go func() {
		t := time.NewTicker(every)
		defer t.Stop()

		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if !p.paused.Load() {
					h.node.CommitBlock()
				}
			}
		}
	}()
	h.t.Cleanup(func() { close(stop) })

	return p
}

// realOptions configures a broadcaster for the scaled window on the system clock.
func (h *harness) realOptions() Options {
	o := h.options()
	o.RefreshInterval = timing.window / 40
	o.WaitLimit = 20 * timing.window
	o.ShutdownGrace = timing.window / 10

	return o
}

// awaitWindowStart waits until the node has committed a block of a new window, and returns that window.
func (h *harness) awaitWindowStart() int64 {
	h.t.Helper()

	w := WindowOf(time.Now().Add(h.lead)) + 1
	time.Sleep(time.Until(WindowStart(w).Add(-h.lead)))
	require.Eventually(h.t, func() bool { return WindowOf(h.snapshot().Time) == w }, timing.window/4, time.Millisecond)

	return w
}

// voidedPair returns two votes the scanner plans, of distinct transfers, that share a slot.
func (h *harness) voidedPair() []sinkid.ID {
	h.t.Helper()

	bySlot := map[int]sinkid.ID{}
	for i := range mixVotes {
		key := policy.KeyOf(h.ids[i], voteFor(h.ids[i], sinkv1.OutcomeVoided, 1))
		s := SlotOf(h.ids[i], key)
		if other, ok := bySlot[s]; ok {
			return []sinkid.ID{other, h.ids[i]}
		}
		bySlot[s] = h.ids[i]
	}

	h.t.Fatal("broadcast-mix has no two votes in one slot")

	return nil
}

// settle is a pass's settle in scan.
func (p *proc) settle(ctx context.Context) func() int {
	return func() int {
		p.b.Flush()
		return p.b.Wait(ctx)
	}
}

func TestRestartNoDoubleExecution(t *testing.T) {
	scaleTiming(t, 1.0/60)
	h := newHarness(t, setup{real: true})
	prod := h.produce(timing.window / 20)
	o := h.realOptions()
	ids := h.voidedPair()

	w := h.awaitWindowStart()
	prod.paused.Store(true)
	h.node.Mutate(func(sc *simnode.Scenario) {
		sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultLostResponse}}
	})

	// A sends both votes in one transaction, which is admitted while its response is lost; then A is discarded. Until a
	// re-send is answered, A re-sends those bytes on its refresh ticks (spec §6.5), so every broadcast A makes is that
	// transaction.
	a := h.start(o, h.signer, h.dial())
	require.NoError(t, a.wire(nil).Review(t.Context(), ids, 0))
	a.b.Flush()
	require.Eventually(t, func() bool { return len(h.node.Broadcasts()) > 0 }, 10*time.Second, time.Millisecond)
	a.stop()
	sent := h.broadcasts()
	sentA := sent[0]
	for _, d := range sent[1:] {
		require.Equal(t, sentA.hash(), d.hash(), "A re-sends only its one transaction")
	}
	require.ElementsMatch(t, []string{ids[0].String(), ids[1].String()}, sentA.ids)
	require.Equal(t, w, WindowOf(sentA.timeout))

	// B, with its own scanner, plans one of those votes in the same window.
	b := h.start(o, h.signer, h.dial())
	require.NoError(t, b.wire(nil).Review(t.Context(), ids[:1], 0))
	b.b.Flush()
	var sentB decoded
	require.Eventually(t, func() bool {
		for _, d := range h.broadcasts() {
			if len(d.ids) == 1 && d.ids[0] == ids[0].String() {
				sentB = d
				return true
			}
		}
		return false
	}, 5*time.Second, time.Millisecond)
	require.True(t, sentA.timeout.Equal(sentB.timeout), "the same slot and timeout")
	require.Eventually(t, func() bool { return b.deferred()[ids[0].String()] == reasonSlotBusy }, 5*time.Second, time.Millisecond,
		"B's transaction is refused: A's holds the replay key")

	// In the next window B's review sees the vote and plans nothing.
	prod.paused.Store(false)
	require.Zero(t, b.b.Wait(t.Context()))
	hashes, errs := outcomes(t, b.lines())
	require.Empty(t, hashes)
	require.Empty(t, errs)

	ex := h.executions()
	require.Equal(t, 1, ex[executedVote(ids[0])])
	require.Equal(t, 1, ex[executedVote(ids[1])])
}

func TestTwoProcessesShareSlots(t *testing.T) {
	scaleTiming(t, 1.0/30)
	h := newHarness(t, setup{real: true})
	h.produce(timing.window / 40)
	o := h.realOptions()

	a := h.start(o, h.signer, h.dial())
	b := h.start(o, h.signer, h.dial())
	sa, sb := a.wire(nil), b.wire(nil)

	h.awaitWindowStart()
	var wg sync.WaitGroup
	failures := make([]int, 2)
	for i, run := range []func() int{
		func() int { return sa.Pass(t.Context(), a.settle(t.Context())) },
		func() int { return sb.Pass(t.Context(), b.settle(t.Context())) },
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failures[i] = run()
		}()
	}
	wg.Wait()
	require.Equal(t, []int{0, 0}, failures)

	ex := h.executions()
	for i := range mixVotes {
		require.Equal(t, 1, ex[executedVote(h.ids[i])], "vote %d", i)
	}
	for i := firstVeto; i < headRefund; i++ {
		require.Equal(t, 1, ex[executedVeto(h.ids[i], 1)], "veto %d", i)
	}
	require.Equal(t, 1, ex[sdk.MsgTypeURL(&sinkv1.MsgFinalizeRefund{})+"|"+h.ids[headRefund].String()])
}

func TestVetoAcrossGovernance(t *testing.T) {
	scaleTiming(t, 1.0/60)
	h := newHarness(t, setup{real: true})
	prod := h.produce(timing.window / 20)
	o := h.realOptions()
	id := h.ids[firstVeto]

	w := h.awaitWindowStart()
	prod.paused.Store(true)

	// Round 1: A's veto is admitted but not yet included.
	a := h.start(o, h.signer, h.dial())
	require.NoError(t, a.wire(nil).Review(t.Context(), []sinkid.ID{id}, 0))
	require.Eventually(t, func() bool { return a.entryOf(id) == phaseSent }, 5*time.Second, time.Millisecond)

	// Governance starts round 2 with a new challenge window; the pending veto stays pending.
	h.node.Resolve(id, sinkv1.StatusRefundPending)
	time.Sleep(10 * time.Millisecond)
	resolved := h.snapshot()
	require.Equal(t, w, WindowOf(resolved.Time), "the same window")

	// Round 2: another process plans a veto again in the same window. It shares the round-1 veto's slot and timeout.
	b := h.start(o, h.signer, h.dial())
	require.NoError(t, b.wire(nil).Review(t.Context(), []sinkid.ID{id}, resolved.Height))
	var review struct {
		Messages []struct {
			TypeURL string `json:"typeUrl"`
		} `json:"messages"`
	}
	lines := b.lines()
	require.Len(t, lines, 1)
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &review))
	require.Len(t, review.Messages, 1)
	require.Equal(t, sdk.MsgTypeURL(&sinkv1.MsgVetoVoid{}), review.Messages[0].TypeURL)

	var timeouts []time.Time
	require.Eventually(t, func() bool {
		timeouts = timeouts[:0]
		for _, raw := range h.node.Broadcasts() {
			timeouts = append(timeouts, decodeTx(t, raw).timeout)
		}
		_, deferredB := b.deferred()[id.String()]
		phaseB := b.entryOf(id)
		return len(timeouts) >= 2 && (deferredB || phaseB == phaseSent)
	}, 5*time.Second, time.Millisecond)
	for _, timeout := range timeouts {
		require.True(t, timeouts[0].Equal(timeout), "one replay key")
	}

	prod.paused.Store(false)
	require.Zero(t, a.b.Wait(t.Context())+b.b.Wait(t.Context()))

	ex := h.executions()
	require.Equal(t, 1, ex[executedVeto(id, 2)], "the round-1 veto executes in round 2, once")
	require.Zero(t, ex[executedVeto(id, 1)])
}

// entryOf reads the phase of the slot transaction carrying a message of id; "" when there is none.
func (p *proc) entryOf(id sinkid.ID) string {
	var phase string
	p.state(func(s *ownerState) {
		for _, e := range s.entries {
			if e.tx == nil {
				continue
			}
			for _, m := range e.tx.msgs {
				if sinkid.Compare(m.ID, id) == 0 {
					phase = phaseOf(e.tx)
				}
			}
		}
	})

	return phase
}

func TestNoVoteExecutesTwice(t *testing.T) {
	scaleTiming(t, 1.0/240)

	rounds := 20
	if os.Getenv("REPORTER_PERF") == "1" {
		rounds = 200
	}

	seed := uint64(time.Now().UnixNano()) //nolint:gosec // a test seed
	if s := os.Getenv("REPORTER_SEED"); s != "" {
		var err error
		seed, err = strconv.ParseUint(s, 10, 64)
		require.NoError(t, err)
	}
	t.Logf("seed %d; replay with REPORTER_SEED=%d", seed, seed)

	began := time.Now()
	for round := range rounds {
		t.Run(fmt.Sprintf("round %d", round), func(t *testing.T) {
			noRepeatRound(t, rand.New(rand.NewPCG(seed, uint64(round)))) //nolint:gosec // a test seed
		})
	}
	t.Logf("%d rounds in %s", rounds, time.Since(began).Round(time.Millisecond))
}

// rival is another process sharing the reporter key: its own scanner plans, and each plan goes out as a transaction
// that other nodes accepted, with its own gas, so its bytes differ from the reporter's.
type rival struct {
	h   *harness
	rng *rand.Rand
	mu  sync.Mutex
}

func (r *rival) Queue(p policy.Planned) bool {
	r.mu.Lock()
	gas := 100_000 + r.rng.Uint64N(50_000)
	r.mu.Unlock()

	timeout := SlotTimeout(WindowOf(p.PlanTime), SlotOf(p.ID, p.Key))
	raw, _, err := r.h.builder(r.h.signer).Build(context.Background(), []sdk.Msg{p.Msg}, gas, FeeFor(gas, testPrices, nil), timeout)
	if err == nil {
		r.h.node.Gossip(raw)
	}

	return true
}

// noRepeatRound runs one randomized interleaving of spec §11.3 for about three windows: a lagging node, lost
// responses, a restart, a second process, rival transactions, a governance resolution and a toggled transaction index.
func noRepeatRound(t *testing.T, rng *rand.Rand) {
	indexOff := rng.IntN(2) == 0
	faults := make([]simnode.BroadcastFault, 60)
	for i := range faults {
		switch r := rng.IntN(20); {
		case r < 5:
			faults[i].Kind = simnode.FaultLostResponse
		case r < 6:
			faults[i].Kind = simnode.FaultTransport
		}
	}

	h := newHarness(t, setup{real: true, change: func(sc *simnode.Scenario) {
		sc.Akash.NoTxIndex = indexOff
		sc.Akash.BroadcastFaults = faults
	}})
	h.produce(timing.window / 20)
	o := h.realOptions()

	// run starts a process that passes in start mode until stopped.
	run := func() (stop func()) {
		p := h.start(o, h.signer, h.dial())
		s := p.wire(nil)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		pause := timing.window / time.Duration(2+rng.IntN(6))

		go func() {
			defer close(done)
			for ctx.Err() == nil {
				s.Pass(ctx, func() int { p.b.Flush(); return p.b.TakeFailures() })
				select {
				case <-ctx.Done():
				case <-time.After(pause):
				}
			}
		}()

		return func() {
			cancel()
			<-done
			p.stop()
		}
	}

	stopA := run()
	stopB := run()

	riv := &rival{h: h, rng: rand.New(rand.NewPCG(rng.Uint64(), rng.Uint64()))} //nolint:gosec // a test seed
	rivalEvents := events.NewWriter(&output{})
	t.Cleanup(func() { _ = rivalEvents.Close() })
	rivalScan := h.scanner(h.dial(), rivalEvents, riv, o.RefreshInterval)

	restartAt := 2 + rng.IntN(10)
	resolveAt := 2 + rng.IntN(20)
	rivalAt := rng.IntN(25)
	vetoed := h.ids[firstVeto+rng.IntN(headRefund-firstVeto)]

	for step := range 30 {
		h.node.SetLag(int64(rng.IntN(4)))

		switch step {
		case restartAt:
			stopA()
			if rng.IntN(2) == 0 {
				indexOff = !indexOff
				h.node.Mutate(func(sc *simnode.Scenario) { sc.Akash.NoTxIndex = indexOff })
			}
			stopA = run()
		case resolveAt:
			h.node.Resolve(vetoed, sinkv1.StatusRefundPending)
		case rivalAt:
			_ = rivalScan.Pass(t.Context(), nil)
		}

		time.Sleep(timing.window / 10)
	}

	stopA()
	stopB()
	h.node.SetLag(0)

	// The round must have exercised something: every vote and veto executed, and the reporters' own broadcasts met
	// injected faults. The rival only gossips, so its transactions consume no fault.
	ex := h.executions()
	votes := 0
	for key := range ex {
		if strings.HasPrefix(key, sdk.MsgTypeURL(&sinkv1.MsgReportOutcome{})+"|") {
			votes++
		}
	}
	require.Equal(t, mixVotes, votes, "every vote executed")
	// Round 2 exists only for the resolved transfer.
	for i := firstVeto; i < headRefund; i++ {
		id := h.ids[i]
		require.Positive(t, ex[executedVeto(id, 1)]+ex[executedVeto(id, 2)], "veto %d", i)
	}

	consumed, fired := 0, 0
	h.node.Mutate(func(sc *simnode.Scenario) { consumed = len(faults) - len(sc.Akash.BroadcastFaults) })
	for _, f := range faults[:consumed] {
		if f.Kind != simnode.FaultNone {
			fired++
		}
	}
	require.Positive(t, fired, "no injected fault fired: the reporters made %d broadcasts", consumed)
}
