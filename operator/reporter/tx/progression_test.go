package tx

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/scan"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// These tests run refund-head progression (spec §6.6) through a real scanner over refund-queue, whose refunds R1, R2
// and R3 are h.ids[0], [1] and [2], in queue order. They run on the system clock with the nonce scheme scaled down.

// headReviewer records the minimum heights of the refund-head reviews it passes on; before, when set, runs ahead of
// each of them.
type headReviewer struct {
	Reviewer

	mu      sync.Mutex
	heights []int64
	before  func(minHeight int64)
}

func (r *headReviewer) ReviewRefundHead(ctx context.Context, minHeight int64) error {
	r.mu.Lock()
	r.heights = append(r.heights, minHeight)
	before := r.before
	r.mu.Unlock()

	if before != nil {
		before(minHeight)
	}

	return r.Reviewer.ReviewRefundHead(ctx, minHeight)
}

func (r *headReviewer) calls() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]int64(nil), r.heights...)
}

// refundQueue serves refund-queue, edited by change, with a block every twentieth of the scaled window, and starts a
// broadcaster whose scanner reviews through a headReviewer.
func refundQueue(t *testing.T, change func(*simnode.Scenario)) (*harness, *proc, *headReviewer, *producer) {
	t.Helper()

	scaleTiming(t, 1.0/20)
	h := newHarness(t, setup{real: true, scenario: "refund-queue", change: change})
	prod := h.produce(timing.window / 20)
	p := h.start(h.realOptions(), h.signer, h.dial())
	rev := &headReviewer{}
	p.wire(func(s *scan.Scanner) Reviewer { rev.Reviewer = s; return rev })

	return h, p, rev, prod
}

func finalizeMsg(id sinkid.ID) *sinkv1.MsgFinalizeRefund {
	return &sinkv1.MsgFinalizeRefund{Executor: simnode.Reporter, ID: id.Proto()}
}

func executedRefund(id sinkid.ID) string {
	return sdk.MsgTypeURL(&sinkv1.MsgFinalizeRefund{}) + "|" + id.String()
}

// refundTx is a broadcast refund transaction: its transfer, its timeout's window and, when it executed, its height.
type refundTx struct {
	id     string
	window int64
	height int64
}

// refundTxs returns the refund transactions broadcast so far, in order.
func (h *harness) refundTxs() []refundTx {
	h.t.Helper()

	var out []refundTx
	for _, d := range h.broadcasts() {
		if _, ok := d.msgs[0].(*sinkv1.MsgFinalizeRefund); !ok {
			continue
		}

		sum := sha256.Sum256(d.raw)
		r, err := h.chain.Tx(h.t.Context(), sum[:])
		require.NoError(h.t, err)

		tx := refundTx{id: d.ids[0], window: WindowOf(d.timeout)}
		if r != nil && r.Code == 0 {
			tx.height = r.Height
		}
		out = append(out, tx)
	}

	return out
}

// executed keeps the refund transactions that executed.
func executed(txs []refundTx) []refundTx {
	var out []refundTx
	for _, tx := range txs {
		if tx.height > 0 {
			out = append(out, tx)
		}
	}

	return out
}

// reviewMessages returns, per transfer, the number of messages of each review event among lines, in order.
func reviewMessages(t *testing.T, lines []string) map[string][]int {
	t.Helper()

	out := map[string][]int{}
	for _, l := range lines {
		var r struct {
			ID       string            `json:"id"`
			Mode     string            `json:"mode"`
			Messages []json.RawMessage `json:"messages"`
		}
		require.NoError(t, json.Unmarshal([]byte(l), &r), l)

		if r.Mode != "" {
			out[r.ID] = append(out[r.ID], len(r.Messages))
		}
	}

	return out
}

func TestRefundHeadProgression(t *testing.T) {
	h, p, rev, _ := refundQueue(t, nil)

	slots := map[int]bool{}
	for _, id := range h.ids {
		slots[SlotOf(id, policy.KeyOf(id, finalizeMsg(id)))] = true
	}
	require.Len(t, slots, 3, "the refunds use distinct slots")

	h.awaitWindowStart()
	require.Zero(t, p.scanner.Pass(t.Context(), p.settle(t.Context())))

	got := executed(h.refundTxs())
	require.Len(t, got, 3)
	for i, tx := range got {
		require.Equal(t, h.ids[i].String(), tx.id, "paid in queue order")
		if i > 0 {
			require.Greater(t, tx.height, got[i-1].height, "one refund per block")
		}
	}
	require.Equal(t, []int64{got[0].height, got[1].height, got[2].height}, rev.calls(),
		"each inclusion reviews the refund head from its own height")

	ex := h.executions()
	for _, id := range h.ids {
		require.Equal(t, 1, ex[executedRefund(id)], id.String())
	}

	hashes, errs := outcomes(t, p.lines())
	require.Len(t, hashes, 3)
	require.Empty(t, errs)
}

func TestProgressionRetriesLowerSnapshot(t *testing.T) {
	h, p, rev, prod := refundQueue(t, nil)
	h.awaitWindowStart()
	prod.paused.Store(true)

	// The refund-head review after R1's inclusion at H starts against a node that serves H-1.
	var lagged atomic.Bool
	rev.mu.Lock()
	rev.before = func(int64) {
		if lagged.CompareAndSwap(false, true) {
			h.node.SetLag(1)
			// The tracker's snapshot from before the lag expires; it is kept for 5 ms.
			time.Sleep(50 * time.Millisecond)
		}
	}
	rev.mu.Unlock()

	failures := make(chan int, 1)
	go func() { failures <- p.scanner.Pass(t.Context(), p.settle(t.Context())) }()

	h.waitBroadcasts(1)
	h.node.CommitBlock()
	require.Eventually(t, func() bool { return len(rev.calls()) == 1 }, 5*time.Second, time.Millisecond)
	head := rev.calls()[0]

	// The review waits for a snapshot at H instead of reviewing R1 again at H-1, and Wait waits for it.
	time.Sleep(4 * p.o.RefreshInterval)
	require.Len(t, h.node.Broadcasts(), 1)
	require.Len(t, reviewMessages(t, p.lines())[h.ids[0].String()], 1, "R1 is reviewed only by the pass")
	select {
	case <-failures:
		t.Fatal("Wait returned while the refund-head review waited")
	default:
	}

	// Once the node serves H, the review finalizes R2.
	h.node.CommitBlock()
	got := h.waitBroadcasts(2)
	require.Equal(t, []string{h.ids[1].String()}, got[1].ids)
	h.node.SetLag(0)
	prod.paused.Store(false)

	select {
	case n := <-failures:
		require.Zero(t, n)
	case <-time.After(10 * time.Second):
		t.Fatal("the pass did not end")
	}

	txs := executed(h.refundTxs())
	require.Len(t, txs, 3)
	require.Equal(t, txs[0].height, head, "the review needs R1's inclusion height")
	ex := h.executions()
	for _, id := range h.ids {
		require.Equal(t, 1, ex[executedRefund(id)], id.String())
	}
}

func TestRefundSlotCollisionWaitsForNextWindow(t *testing.T) {
	h, p, _, _ := refundQueue(t, nil)
	w := h.awaitWindowStart()

	// Another transaction of this reporter already holds R2's slot in window w; executed before R2 is the head, it
	// fails and changes nothing, but its replay key stays.
	r2 := h.ids[1]
	timeout := SlotTimeout(w, SlotOf(r2, policy.KeyOf(r2, finalizeMsg(r2))))
	rival, _, err := h.builder(h.signer).Build(t.Context(), []sdk.Msg{finalizeMsg(r2)}, 123_456, sdk.NewCoins(sdk.NewInt64Coin("uakt", 9)), timeout)
	require.NoError(t, err)
	h.node.Gossip(rival)

	require.Zero(t, p.scanner.Pass(t.Context(), p.settle(t.Context())))

	txs := h.refundTxs()
	got := executed(txs)
	require.Len(t, got, 3)
	require.Equal(t, []refundTx{
		{id: h.ids[0].String(), window: w, height: got[0].height},
		{id: r2.String(), window: w + 1, height: got[1].height},
		{id: h.ids[2].String(), window: got[2].window, height: got[2].height},
	}, got, "R2 is finalized in the next window")
	require.Contains(t, txs, refundTx{id: r2.String(), window: w}, "R2's first send in window w found its slot taken")

	ex := h.executions()
	for _, id := range h.ids {
		require.Equal(t, 1, ex[executedRefund(id)], id.String())
	}
}

// queuedHeads holds each refund-head review until a result arrives on release.
type queuedHeads struct {
	Reviewer

	release chan error
	mu      sync.Mutex
	heights []int64
	active  atomic.Int64
	most    atomic.Int64
}

func (r *queuedHeads) ReviewRefundHead(_ context.Context, minHeight int64) error {
	storeMax(&r.most, int(r.active.Add(1)))
	defer r.active.Add(-1)

	r.mu.Lock()
	r.heights = append(r.heights, minHeight)
	r.mu.Unlock()

	return <-r.release
}

func (r *queuedHeads) calls() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]int64(nil), r.heights...)
}

func TestRefundHeadReviewsCoalesce(t *testing.T) {
	h := newHarness(t, setup{})
	p := h.start(h.options(), h.signer, nil)
	rev := &queuedHeads{release: make(chan error)}
	p.b.SetReviewer(rev)

	included := func(height int64) { require.True(t, p.b.call(func() { p.b.refundIncluded(height) })) }
	idle := func() bool {
		var idle bool
		require.True(t, p.b.call(func() { idle = p.b.idle() }))
		return idle
	}
	reviews := func(n int) {
		t.Helper()
		require.Eventually(t, func() bool { return len(rev.calls()) == n }, 5*time.Second, time.Millisecond)
	}

	// Inclusions while a review runs become one review at the highest height.
	included(10)
	reviews(1)
	included(12)
	included(11)
	require.False(t, idle(), "a review waits to start")
	rev.release <- nil
	reviews(2)
	rev.release <- nil
	require.Eventually(t, idle, 5*time.Second, time.Millisecond)
	require.Equal(t, []int64{10, 12}, rev.calls())

	// A failed review waits for the next inclusion or pass, without holding Wait.
	included(13)
	reviews(3)
	rev.release <- errors.New("snapshot unavailable")
	require.Eventually(t, idle, 5*time.Second, time.Millisecond)
	for range 3 {
		h.tick()
	}
	require.Len(t, rev.calls(), 3, "no retry before the next inclusion or pass")

	p.b.Flush()
	reviews(4)
	rev.release <- errors.New("snapshot unavailable")
	require.Eventually(t, idle, 5*time.Second, time.Millisecond)
	included(14)
	reviews(5)
	rev.release <- nil
	require.Eventually(t, idle, 5*time.Second, time.Millisecond)

	require.Equal(t, []int64{10, 12, 13, 13, 14}, rev.calls())
	require.Equal(t, int64(1), rev.most.Load(), "never two refund-head reviews at once")
}

func TestProgressionStops(t *testing.T) {
	// R2, the head after R1, is still inside its challenge window, so it plans nothing; R3 queues behind it.
	h, p, rev, _ := refundQueue(t, func(sc *simnode.Scenario) {
		sc.Akash.Transfers[1].Transfer.Settlement.ChallengeEnd = 4_000_000_000
		sc.Akash.Transfers[2].Transfer.Settlement.ChallengeEnd = 4_000_000_001
	})
	h.awaitWindowStart()

	require.Zero(t, p.scanner.Pass(t.Context(), p.settle(t.Context())))

	txs := h.refundTxs()
	require.Len(t, txs, 1)
	require.Equal(t, h.ids[0].String(), txs[0].id)
	require.Positive(t, txs[0].height)
	require.Equal(t, []int64{txs[0].height}, rev.calls())

	lines := p.lines()
	_, errs := outcomes(t, lines)
	require.Empty(t, errs)
	require.Equal(t, map[string][]int{
		h.ids[0].String(): {1},
		h.ids[1].String(): {0, 0},
		h.ids[2].String(): {0},
	}, reviewMessages(t, lines), "the pass reviews all three; the follow-up reviews R2 and plans nothing")

	ex := h.executions()
	require.Equal(t, 1, ex[executedRefund(h.ids[0])])
	require.Zero(t, ex[executedRefund(h.ids[1])])
	require.Zero(t, ex[executedRefund(h.ids[2])])
}
