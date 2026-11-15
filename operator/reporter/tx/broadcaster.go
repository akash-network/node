package tx

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/metrics"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/scan"
	"pkg.akt.dev/node/v2/operator/reporter/signer"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

const (
	defaultMaxMsgs         = 25
	defaultMaxInFlight     = 8
	defaultRefreshInterval = time.Second
	defaultWaitLimit       = 3 * time.Minute
	defaultShutdownGrace   = 30 * time.Second
	opsBuffer              = 64

	errRemoved = "reporter was removed"
)

// Chain is what the broadcaster reads and sends through: the methods of *akash.Client it uses.
type Chain interface {
	Latest(ctx context.Context) (*akash.State, error)
	Simulate(ctx context.Context, txBytes []byte) (uint64, error)
	BroadcastSync(ctx context.Context, txBytes []byte) (akash.BroadcastResult, error)
	Tx(ctx context.Context, hash []byte) (*akash.TxResult, error)
	TxIndexEnabled() bool
}

var _ Chain = (*akash.Client)(nil)

// Reviewer runs the follow-up reviews of spec §6.6; *scan.Scanner satisfies it.
type Reviewer interface {
	ReviewRefundHead(ctx context.Context, minHeight int64) error
	Review(ctx context.Context, ids []sinkid.ID, minHeight int64) error
}

var _ Reviewer = (*scan.Scanner)(nil)

// Options configures a Broadcaster.
type Options struct {
	// Reporter is the policy member whose key signs.
	Reporter    string
	MaxMsgs     int
	MaxInFlight int
	// GasPerMessage is the numeric gas setting; zero means gas = auto.
	GasPerMessage uint64
	GasAdjustment float64
	GasPrices     sdk.DecCoins
	Fees          sdk.Coins
	MaxGas        uint64
	MaxFee        sdk.Coin
	// SignerTimeout is the remote signer's timeout, 0 for the keyring; it lengthens the send cutoff.
	SignerTimeout time.Duration
	// RefreshInterval paces snapshot refreshes, re-sends and retries; zero means one second.
	RefreshInterval time.Duration
	// WaitLimit is how long Wait waits without progress; zero means three minutes.
	WaitLimit time.Duration
	// ShutdownGrace is how long Run waits for sent transactions after its context ends; zero means 30 s.
	ShutdownGrace time.Duration
	// Clock drives timers only; chain time comes from snapshots. Nil means the system clock.
	Clock   clock.Clock
	Metrics *metrics.Metrics

	// changed, set only by tests, sees the owner's state after every change, on the owner goroutine.
	changed func(*ownerState)
}

// Broadcaster sends planned messages as batched unordered transactions under the nonce-slot rules of spec §6. One
// owner goroutine, Run's, holds the slot table and writes outcome events; workers do every request.
type Broadcaster struct {
	o     Options
	chain Chain
	b     *Builder
	ev    *events.Writer

	ops      chan func()
	done     chan struct{}
	follow   chan followJob
	failures atomic.Int64

	reviewerMu sync.Mutex
	reviewer   Reviewer

	// Run sets these before it starts any goroutine. stop is Run's context: once it ends, nothing new is signed. work
	// carries requests and outlives stop by the shutdown grace.
	stop context.Context
	work context.Context
	log  *zerolog.Logger
	wg   sync.WaitGroup

	// st is only read or written on the owner goroutine.
	st ownerState
}

type ownerState struct {
	table

	// chainTime is the latest block time seen in any snapshot or plan; height is the highest height seen.
	chainTime time.Time
	height    int64
	// window is chainTime's window, and windowHeight the height of the first block seen in it.
	window       int64
	windowHeight int64

	inFlight   int
	refreshing bool
	indexed    bool
	stopping   bool

	followBusy bool
	// followJobs numbers the follow-up reviews.
	followJobs uint64
	// followFailed is the window of the last follow-up that failed; none is retried before a later window.
	followFailed int64
	// refundAt is the highest inclusion height of a finalized refund whose refund-head review has not started; zero
	// means none. refundHeld keeps it from starting after a failed review, until the next inclusion or pass.
	refundAt   int64
	refundHeld bool

	// progress is when the last submission, outcome or new window happened.
	progress time.Time
	// notify is closed after every change.
	notify chan struct{}
}

type followJob struct {
	// id tells the batches this review fills from the others; it is never passFiller.
	id uint64
	// refund marks a review of the refund head; the others review deferred transfers.
	refund    bool
	ids       []sinkid.ID
	keys      []policy.Key
	window    int64
	minHeight int64
}

// New returns a broadcaster; Run starts it.
func New(o Options, chain Chain, b *Builder, ev *events.Writer) *Broadcaster {
	if o.MaxMsgs <= 0 {
		o.MaxMsgs = defaultMaxMsgs
	}
	if o.MaxInFlight <= 0 {
		o.MaxInFlight = defaultMaxInFlight
	}
	if o.GasAdjustment <= 0 {
		o.GasAdjustment = 1
	}
	if o.RefreshInterval <= 0 {
		o.RefreshInterval = defaultRefreshInterval
	}
	if o.WaitLimit <= 0 {
		o.WaitLimit = defaultWaitLimit
	}
	if o.ShutdownGrace <= 0 {
		o.ShutdownGrace = defaultShutdownGrace
	}
	if o.Clock == nil {
		o.Clock = clock.Real()
	}

	return &Broadcaster{
		o:      o,
		chain:  chain,
		b:      b,
		ev:     ev,
		ops:    make(chan func(), opsBuffer),
		done:   make(chan struct{}),
		follow: make(chan followJob, 1),
		st: ownerState{
			table:        newTable(),
			window:       math.MinInt64,
			followFailed: math.MinInt64,
			notify:       make(chan struct{}),
		},
	}
}

// SetReviewer sets the scanner that runs follow-up reviews.
func (b *Broadcaster) SetReviewer(r Reviewer) {
	b.reviewerMu.Lock()
	defer b.reviewerMu.Unlock()

	b.reviewer = r
}

func (b *Broadcaster) getReviewer() Reviewer {
	b.reviewerMu.Lock()
	defer b.reviewerMu.Unlock()

	return b.reviewer
}

// post hands f to the owner goroutine; it reports false once Run has returned.
func (b *Broadcaster) post(f func()) bool {
	select {
	case b.ops <- f:
		return true
	case <-b.done:
		return false
	}
}

// call runs f on the owner goroutine and waits for it; it reports false once Run has returned.
func (b *Broadcaster) call(f func()) bool {
	ran := make(chan struct{})
	if !b.post(func() { f(); close(ran) }) {
		return false
	}

	select {
	case <-ran:
		return true
	case <-b.done:
		return false
	}
}

// Queue offers a planned message. It reports false when the message was skipped, because its key is already queued
// or sent in its slot, or deferred to a later window. It is safe for concurrent use and never waits for signing.
func (b *Broadcaster) Queue(p policy.Planned) bool { return b.queueFrom(p, passFiller) }

func (b *Broadcaster) queueFrom(p policy.Planned, filler uint64) bool {
	var ok bool
	if !b.call(func() { ok = b.queue(p, filler) }) {
		return false
	}

	return ok
}

// followQueuer takes the plans of one follow-up review, so that the batches it fills go out when it ends.
type followQueuer struct {
	b   *Broadcaster
	job uint64
}

func (q followQueuer) Queue(p policy.Planned) bool { return q.b.queueFrom(p, q.job) }

// Flush reports that the pass feeding Queue has ended: the partial vote batches it filled go out now, except those a
// follow-up review is still filling. A refund-head review that failed is retried.
func (b *Broadcaster) Flush() {
	b.post(func() {
		b.reviewEnded(passFiller)
		b.st.refundHeld = false
	})
}

// TakeFailures returns the failures resolved since the last call.
func (b *Broadcaster) TakeFailures() int { return int(b.failures.Swap(0)) }

// Wait returns once nothing is queued, unresolved or deferred and no follow-up review runs or waits to start, or once
// WaitLimit passes without progress; it then reports what is left (spec §6.5). It also returns when ctx ends. It
// returns the failures resolved since the last TakeFailures.
func (b *Broadcaster) Wait(ctx context.Context) int {
	began := b.o.Clock.Now()

	for {
		var (
			idle    bool
			last    time.Time
			changed <-chan struct{}
		)
		if !b.call(func() { idle, last, changed = b.idle(), b.st.progress, b.st.notify }) || idle {
			return b.TakeFailures()
		}

		if last.Before(began) {
			last = began
		}

		left := b.o.WaitLimit - b.o.Clock.Now().Sub(last)
		if left <= 0 {
			b.call(b.giveUp)
			return b.TakeFailures()
		}

		select {
		case <-ctx.Done():
			return b.TakeFailures()
		case <-b.done:
			return b.TakeFailures()
		case <-changed:
		case <-b.o.Clock.After(left):
		}
	}
}

// Run owns the slot table until ctx ends, then gives sent transactions up to ShutdownGrace to resolve.
func (b *Broadcaster) Run(ctx context.Context) error {
	b.log = zerolog.Ctx(ctx)
	b.st.indexed = b.chain.TxIndexEnabled()
	if !b.st.indexed {
		b.log.Warn().Msg("the Akash node does not index transactions; transaction outcomes cannot be observed")
	}

	work, cancelWork := context.WithCancel(context.WithoutCancel(ctx))
	b.stop, b.work = ctx, work
	b.st.progress = b.o.Clock.Now()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.followUps(ctx)
	}()

	defer func() {
		close(b.done)
		cancelWork()
		b.wg.Wait()
	}()

	tick := b.o.Clock.After(b.o.RefreshInterval)
	stopping := ctx.Done()
	var grace <-chan time.Time

	for {
		select {
		case f := <-b.ops:
			f()
		case <-tick:
			tick = b.o.Clock.After(b.o.RefreshInterval)
			b.tick()
		case <-stopping:
			stopping = nil
			b.shutdown()
			grace = b.o.Clock.After(b.o.ShutdownGrace)
		case <-grace:
			return nil
		}

		b.pump()

		if b.st.stopping && b.settled() {
			return nil
		}
	}
}

// spawn runs f on a goroutine that Run waits for.
func (b *Broadcaster) spawn(f func()) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		f()
	}()
}

func (b *Broadcaster) progressed() { b.st.progress = b.o.Clock.Now() }

// observe takes a snapshot's or a plan's height and block time into the latest chain time seen.
func (b *Broadcaster) observe(height int64, t time.Time) {
	s := &b.st
	if t.After(s.chainTime) {
		s.chainTime = t
		if w := WindowOf(t); w > s.window {
			s.window, s.windowHeight = w, height
			b.progressed()
		}
	}

	s.height = max(s.height, height)
}

// tick refreshes the snapshot while anything is pending and re-sends transactions whose last broadcast failed.
func (b *Broadcaster) tick() {
	s := &b.st
	if !s.refreshing && (len(s.entries) > 0 || len(s.deferred) > 0 || s.followBusy) {
		s.refreshing = true
		b.spawn(func() {
			st, err := b.chain.Latest(b.work)
			b.post(func() {
				b.st.refreshing = false
				if err == nil {
					b.observe(st.Height, st.Time)
				}
			})
		})
	}

	for _, e := range s.entries {
		if tx := e.tx; tx != nil && tx.phase == sent && tx.resend && !tx.busy && !s.chainTime.After(e.timeout) {
			b.resend(e, tx)
		}
	}
}

// pump runs after every change: deadlines, sends, lookups, window cleanup and follow-ups.
func (b *Broadcaster) pump() {
	s := &b.st

	b.checkQueued()

	if !s.stopping {
		for _, e := range b.sendable() {
			if s.inFlight >= b.o.MaxInFlight {
				break
			}
			b.startBuild(e)
		}
	}

	for _, e := range s.entries {
		if tx := e.tx; tx != nil && tx.phase == sent && tx.observed && !tx.busy && tx.looked < s.height {
			b.lookup(e, tx)
		}
	}

	b.settleWindows()

	if !s.stopping {
		b.startFollowUp()
	}

	if b.o.changed != nil {
		b.o.changed(s)
	}

	close(s.notify)
	s.notify = make(chan struct{})
}

// startBuild hands e's queued messages to a worker as one transaction.
func (b *Broadcaster) startBuild(e *slotEntry) {
	tx := &liveTx{msgs: e.queued, phase: building, busy: true, inFlight: true}
	e.tx = tx
	e.clearQueue()
	b.st.inFlight++
	b.o.Metrics.AddTxInFlight(1)

	job := buildJob{msgs: tx.msgs, timeout: e.timeout, chainTime: b.st.chainTime}
	b.spawn(func() {
		res := b.build(job)
		b.post(func() { b.applyBuild(e, tx, res) })
	})
}

func (b *Broadcaster) lookup(e *slotEntry, tx *liveTx) {
	tx.busy = true
	h, final, hash := b.st.height, b.st.chainTime.After(e.timeout), tx.hash

	b.spawn(func() {
		r, err := b.chain.Tx(b.work, hash)
		if err != nil {
			b.log.Debug().Err(err).Str("hash", fmt.Sprintf("%X", hash)).Msg("transaction lookup failed")
		}
		b.post(func() { b.applyLookup(e, tx, h, final, r, err) })
	})
}

// resend sends a transaction's bytes again after a transport error (spec §6.5).
func (b *Broadcaster) resend(e *slotEntry, tx *liveTx) {
	tx.busy = true
	raw := tx.raw

	b.spawn(func() {
		r, err := b.chain.BroadcastSync(b.work, raw)
		b.post(func() {
			tx.busy = false
			if e.tx != tx || err != nil {
				return
			}

			// Accepted, already in the cache, or refused because an earlier copy holds the replay key: the copy may be
			// pending or included. Any other refusal of a re-send is not final either; lookups or expiry decide.
			tx.resend = false
			if r.InCache || r.Code == 0 {
				b.progressed()
			}
		})
	})
}

// refundIncluded records a finalized refund's inclusion: the refund head is reviewed next (spec §6.6).
func (b *Broadcaster) refundIncluded(height int64) {
	if b.getReviewer() == nil {
		return
	}

	s := &b.st
	s.refundAt = max(s.refundAt, height)
	s.refundHeld = false
}

// startFollowUp starts the next follow-up review (spec §6.6), one at a time: the refund head after a refund's
// inclusion, at the highest inclusion height seen; otherwise, once a block of a later window is seen, the transfers
// deferred in earlier windows.
func (b *Broadcaster) startFollowUp() {
	s := &b.st
	if s.followBusy || b.getReviewer() == nil {
		return
	}

	if s.refundAt > 0 && !s.refundHeld {
		b.startJob(followJob{refund: true, minHeight: s.refundAt})
		s.refundAt = 0
		return
	}

	if s.window <= s.followFailed {
		return
	}

	job := followJob{window: s.window, minHeight: s.windowHeight}
	seen := map[string]bool{}
	for key, d := range s.deferred {
		if d.window >= s.window {
			continue
		}
		job.keys = append(job.keys, key)
		if id := d.planned.ID.String(); !seen[id] {
			seen[id] = true
			job.ids = append(job.ids, d.planned.ID)
		}
	}
	if len(job.ids) == 0 {
		return
	}

	slices.SortFunc(job.ids, sinkid.Compare)
	b.startJob(job)
}

func (b *Broadcaster) startJob(job followJob) {
	s := &b.st
	s.followJobs++
	job.id = s.followJobs
	s.followBusy = true
	b.follow <- job
}

// followUps runs follow-up reviews one at a time until ctx ends.
func (b *Broadcaster) followUps(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-b.follow:
			r, queued := b.getReviewer(), scan.WithQueuer(ctx, followQueuer{b: b, job: job.id})

			var err error
			if job.refund {
				err = r.ReviewRefundHead(queued, job.minHeight)
			} else {
				err = r.Review(queued, job.ids, job.minHeight)
			}

			if err != nil && !errors.Is(err, scan.ErrPassAborted) && ctx.Err() == nil {
				if job.refund {
					b.log.Warn().Err(err).Int64("height", job.minHeight).Msg("refund-head review failed; retrying on the next inclusion or pass")
				} else {
					b.log.Warn().Err(err).Int("transfers", len(job.ids)).Msg("follow-up review failed; retrying in the next window")
				}
			}

			b.post(func() { b.followDone(job, err) })
		}
	}
}

// followDone releases the batches the review filled and drops the deferrals a successful review covered. A failed or
// skipped review keeps them, counting nothing, for the next window; a failed or skipped refund-head review is held
// until the next inclusion or pass.
func (b *Broadcaster) followDone(job followJob, err error) {
	s := &b.st
	s.followBusy = false
	b.reviewEnded(job.id)

	if job.refund {
		// A later inclusion recorded meanwhile supersedes the retry.
		if err != nil && s.refundAt == 0 {
			s.refundAt, s.refundHeld = job.minHeight, true
		}
		return
	}

	if err != nil {
		s.followFailed = job.window
		return
	}

	for _, key := range job.keys {
		if d := s.deferred[key]; d != nil && d.window < job.window {
			delete(s.deferred, key)
		}
	}
}

// inspect runs f on the owner goroutine; tests read the table through it.
func (b *Broadcaster) inspect(f func(*ownerState)) bool { return b.call(func() { f(&b.st) }) }

type buildJob struct {
	msgs      []policy.Planned
	timeout   time.Time
	chainTime time.Time
}

type sendOutcome int

const (
	sendAccepted sendOutcome = iota
	sendTransport
	sendInCache
	sendSlotTaken
	sendRejected
)

type msgFailure struct {
	planned policy.Planned
	text    string
}

type msgDeferral struct {
	planned policy.Planned
	reason  string
}

type firstSend struct {
	msgs    []policy.Planned
	raw     []byte
	hash    []byte
	fee     sdk.Coins
	outcome sendOutcome
	code    uint32
	log     string
}

// buildResult is what a worker decided for a transaction's messages before and at its first broadcast.
type buildResult struct {
	seen     *akash.State
	failed   []msgFailure
	deferred []msgDeferral
	released []policy.Planned
	sent     *firstSend
}

func (r *buildResult) fail(ps []policy.Planned, text string) {
	for _, p := range ps {
		r.failed = append(r.failed, msgFailure{planned: p, text: text})
	}
}

func (r *buildResult) deferAll(ps []policy.Planned, reason string) {
	for _, p := range ps {
		r.deferred = append(r.deferred, msgDeferral{planned: p, reason: reason})
	}
}

func (r *buildResult) see(st *akash.State) {
	if r.seen == nil || st.Time.After(r.seen.Time) {
		r.seen = st
	}
}

// build simulates, prices, caps, signs and first sends one slot's messages (spec §6.2-6.5). It runs on a worker.
func (b *Broadcaster) build(job buildJob) (res buildResult) {
	msgs := job.msgs
	chainTime := job.chainTime
	cutoff := SendCutoff(b.o.SignerTimeout)

	retry := func() bool {
		select {
		case <-b.stop.Done():
			return false
		case <-b.o.Clock.After(b.o.RefreshInterval):
			return true
		}
	}

	for len(msgs) > 0 {
		if b.stop.Err() != nil {
			res.released = append(res.released, msgs...)
			return res
		}

		st, err := b.chain.Latest(b.work)
		if err == nil {
			res.see(st)
			if st.Time.After(chainTime) {
				chainTime = st.Time
			}
		}

		if job.timeout.Sub(chainTime) < cutoff {
			res.deferAll(msgs, reasonCutoff)
			return res
		}

		if err != nil {
			b.log.Debug().Err(err).Msg("snapshot before signing failed; retrying")
			if !retry() {
				res.released = append(res.released, msgs...)
				return res
			}
			continue
		}

		if !slices.Contains(st.Policy.Members, b.o.Reporter) {
			res.fail(msgs, errRemoved)
			return res
		}
		if addr := b.b.signer.Address().String(); addr != b.o.Reporter {
			res.fail(msgs, fmt.Sprintf("signer %s is not reporter %s", addr, b.o.Reporter))
			return res
		}

		pr := Prune(b.simulator(job.timeout), msgs)
		for _, d := range pr.Dropped {
			res.fail([]policy.Planned{d.Planned}, d.Err.Error())
		}
		res.deferAll(pr.Deferred, reasonSimulation)

		if pr.Unavailable != nil {
			msgs = pr.Retry
			b.log.Debug().Err(pr.Unavailable).Msg("simulation unavailable; retrying")
			if !retry() {
				res.released = append(res.released, msgs...)
				return res
			}
			continue
		}

		msgs = pr.Kept
		if len(msgs) == 0 {
			return res
		}

		gas := b.gasLimit(pr.GasUsed, len(msgs))
		fee := FeeFor(gas, b.o.GasPrices, b.o.Fees)

		if over := b.overCap(gas, fee); over != "" {
			msgs = b.halve(&res, msgs, over)
			continue
		}

		if limit := signer.SignBytesLimit(b.b.signer); limit > 0 {
			fit, err := b.fitting(msgs, gas, fee, job.timeout, limit)
			if err != nil {
				res.fail(msgs, err.Error())
				return res
			}

			switch {
			case fit == 0:
				n, _ := b.b.SignBytesLen(sdkMsgs(msgs[:1]), gas, fee, job.timeout)
				res.fail(msgs[:1], fmt.Errorf("%w: %d bytes, limit %d", ErrSignBytesLimit, n, limit).Error())
				msgs = msgs[1:]
				continue
			case fit < len(msgs):
				res.deferAll(msgs[fit:], reasonCap)
				msgs = msgs[:fit]
				continue
			}
		}

		raw, hash, err := b.b.Build(b.stop, sdkMsgs(msgs), gas, fee, job.timeout)
		switch {
		case errors.Is(err, ErrSignBytesLimit):
			msgs = b.halve(&res, msgs, err.Error())
			continue
		case b.stop.Err() != nil:
			res.released = append(res.released, msgs...)
			return res
		case err != nil:
			res.fail(msgs, err.Error())
			return res
		}

		res.sent = b.firstSend(msgs, raw, hash, fee)

		return res
	}

	return res
}

// halve defers the second half of a batch over a cap, or fails a single message (spec §6.4).
func (b *Broadcaster) halve(res *buildResult, msgs []policy.Planned, over string) []policy.Planned {
	if len(msgs) == 1 {
		res.fail(msgs, over)
		return nil
	}

	keep := (len(msgs) + 1) / 2
	res.deferAll(msgs[keep:], reasonCap)

	return msgs[:keep]
}

// firstSend broadcasts a signed transaction for the first time and classifies the answer (spec §6.5).
func (b *Broadcaster) firstSend(msgs []policy.Planned, raw, hash []byte, fee sdk.Coins) *firstSend {
	out := &firstSend{msgs: msgs, raw: raw, hash: hash, fee: fee}

	r, err := b.chain.BroadcastSync(b.work, raw)
	switch {
	case err != nil:
		b.log.Debug().Err(err).Str("hash", fmt.Sprintf("%X", hash)).Msg("broadcast failed; re-sending")
		out.outcome = sendTransport
	case r.InCache:
		out.outcome = sendInCache
	case r.Code == 0:
		out.outcome = sendAccepted
	case strings.Contains(r.Log, nonceTaken):
		out.outcome = sendSlotTaken
	default:
		out.outcome, out.code, out.log = sendRejected, r.Code, r.Log
	}

	return out
}

// simulator simulates a batch with the slot's timeout. A transaction that cannot be encoded is the batch's own
// failure, not a transport error.
func (b *Broadcaster) simulator(timeout time.Time) func([]sdk.Msg) (uint64, error) {
	return func(msgs []sdk.Msg) (uint64, error) {
		gas, fee := b.simShape(len(msgs))
		bz, err := b.b.SimBytes(msgs, gas, fee, timeout)
		if err != nil {
			return 0, status.Error(codes.Internal, err.Error())
		}

		return b.chain.Simulate(b.work, bz)
	}
}

// simShape returns the gas limit and fee to simulate n messages with. Known before simulating, they are exact.
// With gas = auto the limit is tx.max-gas, whose encoding is at least as long as the final limit's, and a fee
// priced from gas prices is one unit of each denomination: the fee deduction is simulated, and a reporter whose
// balance covers the real fee never fails simulation on it.
func (b *Broadcaster) simShape(n int) (uint64, sdk.Coins) {
	if b.o.GasPerMessage > 0 {
		gas := b.gasLimit(0, n)
		return gas, FeeFor(gas, b.o.GasPrices, b.o.Fees)
	}

	if !b.o.Fees.IsZero() {
		return b.o.MaxGas, b.o.Fees
	}

	fee := make([]sdk.Coin, 0, len(b.o.GasPrices))
	for _, price := range b.o.GasPrices {
		fee = append(fee, sdk.NewInt64Coin(price.Denom, 1))
	}

	return b.o.MaxGas, sdk.NewCoins(fee...)
}

// gasLimit is GasPerMessage × n, or with gas = auto ceil(used × GasAdjustment) (spec §6.4).
func (b *Broadcaster) gasLimit(used uint64, n int) uint64 {
	if b.o.GasPerMessage > 0 {
		return b.o.GasPerMessage * uint64(n) //nolint:gosec // n is a batch size
	}

	adjustment, err := sdkmath.LegacyNewDecFromStr(strconv.FormatFloat(b.o.GasAdjustment, 'f', -1, 64))
	if err != nil {
		adjustment = sdkmath.LegacyOneDec()
	}

	return sdkmath.LegacyNewDecFromInt(sdkmath.NewIntFromUint64(used)).Mul(adjustment).Ceil().TruncateInt().Uint64()
}

// overCap describes a gas limit or fee above its cap, or returns "".
func (b *Broadcaster) overCap(gas uint64, fee sdk.Coins) string {
	if b.o.MaxGas > 0 && gas > b.o.MaxGas {
		return fmt.Sprintf("gas limit %d exceeds tx.max-gas %d", gas, b.o.MaxGas)
	}

	if capped := b.o.MaxFee; !capped.Amount.IsNil() && fee.AmountOf(capped.Denom).GT(capped.Amount) {
		return fmt.Sprintf("fee %s exceeds tx.max-fee %s", fee, capped)
	}

	return ""
}

// fitting returns how many leading messages fit the signer's sign-bytes limit.
func (b *Broadcaster) fitting(msgs []policy.Planned, gas uint64, fee sdk.Coins, timeout time.Time, limit int) (int, error) {
	lo, hi := 0, len(msgs)
	for lo < hi {
		mid := (lo + hi + 1) / 2

		n, err := b.b.SignBytesLen(sdkMsgs(msgs[:mid]), gas, fee, timeout)
		if err != nil {
			return 0, err
		}

		if n <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}

	return lo, nil
}

func sdkMsgs(ps []policy.Planned) []sdk.Msg {
	out := make([]sdk.Msg, len(ps))
	for i, p := range ps {
		out[i] = p.Msg
	}

	return out
}
