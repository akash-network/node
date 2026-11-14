package tx

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// Deferral reasons, the reason label of akt_reporter_deferred_messages_total (spec §9.3).
const (
	reasonSlotBusy   = "slot_busy"
	reasonSlotFull   = "slot_full"
	reasonCutoff     = "cutoff"
	reasonSimulation = "simulation"
	reasonCap        = "cap"
)

// Transaction results, the result label of akt_reporter_tx_total.
const (
	resultIncluded    = "included"
	resultFailed      = "failed"
	resultRejected    = "rejected"
	resultSlotTaken   = "slot_taken"
	resultExpired     = "expired"
	resultUnconfirmed = "unconfirmed"
)

const (
	// nonceTaken is the replay-key refusal, at admission and in a block (spec §6.5).
	nonceTaken = "failed to add unordered nonce"
	// replayCode is sdkerrors.ErrInvalidRequest, the code of a replay-key failure in a block.
	replayCode      = 18
	replayCodespace = "sdk"
	feeDenom        = "uakt"
)

// slotKey identifies a nonce slot in one window.
type slotKey struct {
	window int64
	slot   int
}

type txPhase int

const (
	// building: a worker simulates, signs and sends the transaction for the first time.
	building txPhase = iota
	// sent: the transaction was broadcast at least once and may hold the slot's replay key.
	sent
)

// liveTx is the one live transaction of a slot in its window.
type liveTx struct {
	msgs  []policy.Planned
	phase txPhase
	raw   []byte
	hash  []byte
	fee   sdk.Coins
	// resend is set while the last broadcast failed in transport: the same bytes go out again on each refresh until
	// the timeout passes.
	resend bool
	// busy is set while a worker runs a step of this transaction.
	busy bool
	// looked is the highest height whose lookup has answered.
	looked int64
	// observed means lookups await the outcome; it is false without a transaction index, and once Wait gave up.
	observed bool
	// inFlight means the transaction counts against MaxInFlight.
	inFlight bool
	// abandoned means Wait reported the messages of this transaction, still being built, as not sent: nothing more is
	// written for them, and a transaction the build sends anyway keeps its slot without being observed.
	abandoned bool
}

// slotEntry is a slot in one window: the messages queued for it, then the transaction built from them.
type slotEntry struct {
	key     slotKey
	timeout time.Time
	kind    policy.MsgKind
	// seq orders entries by their first queued message, which is plan order.
	seq    uint64
	queued []policy.Planned
	// filling holds the reviews still queuing into the batch: passFiller, or a follow-up's job number. A partial vote
	// batch goes out once every review that filled it has ended, or at its flush lead (spec §6.2).
	filling map[uint64]struct{}
	// lead marks a vote batch that has reached its flush lead.
	lead bool
	// keys holds every message key queued or sent in the slot this window.
	keys map[policy.Key]struct{}
	tx   *liveTx
	// closed means the slot's replay key is used in this window, by this process or another.
	closed bool
}

func (e *slotEntry) drop(p policy.Planned) { delete(e.keys, p.Key) }

// clearQueue empties the batch, which no review fills any more.
func (e *slotEntry) clearQueue() {
	e.queued, e.lead = nil, false
	clear(e.filling)
}

// reviewEnded records that filler has stopped queuing: batches it alone was still filling can go out.
func (b *Broadcaster) reviewEnded(filler uint64) {
	for _, e := range b.st.entries {
		delete(e.filling, filler)
	}
}

// deferral is a message that waits for a review in a later window.
type deferral struct {
	planned policy.Planned
	reason  string
	window  int64
	until   time.Time
}

// table is the slot table of spec §6.5, keyed by window and slot, and the deferred messages, keyed by message key.
type table struct {
	entries  map[slotKey]*slotEntry
	deferred map[policy.Key]*deferral
	seq      uint64
}

func newTable() table {
	return table{entries: map[slotKey]*slotEntry{}, deferred: map[policy.Key]*deferral{}}
}

// capacity is how many messages one transaction of kind carries.
func (b *Broadcaster) capacity(kind policy.MsgKind) int {
	if kind == policy.MsgVote {
		return b.o.MaxMsgs
	}

	return 1
}

// passFiller is the review behind a plain Queue: a pass, whose end Flush reports.
const passFiller = 0

// queue admits a message that the review filler planned to its slot, or skips or defers it (spec §6.5).
func (b *Broadcaster) queue(p policy.Planned, filler uint64) bool {
	s := &b.st
	if s.stopping {
		return false
	}

	b.observe(p.PlanHeight, p.PlanTime)

	k := slotKey{window: WindowOf(p.PlanTime), slot: SlotOf(p.ID, p.Key)}
	timeout := SlotTimeout(k.window, k.slot)

	e := s.entries[k]
	if e != nil {
		if _, dup := e.keys[p.Key]; dup {
			return false
		}
	}

	if k.window < s.window || timeout.Sub(s.chainTime) < SendCutoff(b.o.SignerTimeout) {
		b.deferMsg(p, reasonCutoff, k.window, timeout)
		return false
	}

	if e == nil {
		e = &slotEntry{key: k, timeout: timeout, kind: p.Key.Kind, keys: map[policy.Key]struct{}{}, filling: map[uint64]struct{}{}}
		s.entries[k] = e
	}

	switch {
	case e.closed || e.tx != nil:
		b.deferMsg(p, reasonSlotBusy, k.window, timeout)
		return false
	case len(e.queued) >= b.capacity(e.kind):
		b.deferMsg(p, reasonSlotFull, k.window, timeout)
		return false
	}

	if len(e.queued) == 0 {
		s.seq++
		e.seq = s.seq
	}

	e.queued = append(e.queued, p)
	e.keys[p.Key] = struct{}{}
	e.filling[filler] = struct{}{}
	delete(s.deferred, p.Key)

	return true
}

func (b *Broadcaster) deferMsg(p policy.Planned, reason string, window int64, until time.Time) {
	b.st.deferred[p.Key] = &deferral{planned: p, reason: reason, window: window, until: until}
	b.o.Metrics.RecordDeferred(reason)
}

// fail writes a message's error and counts it.
func (b *Broadcaster) fail(p policy.Planned, text string) {
	b.ev.Emit(events.TransferError{ID: p.ID.String(), Error: text})
	b.failures.Add(1)
	b.progressed()
}

// endTx removes the entry's transaction, which no longer counts against MaxInFlight.
func (b *Broadcaster) endTx(e *slotEntry) {
	b.leaveFlight(e.tx)
	e.tx = nil
}

func (b *Broadcaster) leaveFlight(tx *liveTx) {
	if tx != nil && tx.inFlight {
		tx.inFlight = false
		b.st.inFlight--
		b.o.Metrics.AddTxInFlight(-1)
	}
}

// checkQueued defers queued messages whose send cutoff has passed and readies vote batches at their flush lead.
func (b *Broadcaster) checkQueued() {
	s := &b.st
	cutoff := SendCutoff(b.o.SignerTimeout)

	for _, e := range s.entries {
		if len(e.queued) == 0 || e.tx != nil {
			continue
		}

		left := e.timeout.Sub(s.chainTime)
		switch {
		case e.key.window < s.window || left < cutoff:
			for _, p := range e.queued {
				e.drop(p)
				b.deferMsg(p, reasonCutoff, e.key.window, e.timeout)
			}
			e.clearQueue()
		case left <= cutoff+timing.flushLead:
			e.lead = true
		}
	}
}

// sendable lists the entries whose messages can go out now, in send priority: vetoes, refunds, then vote batches in
// plan order (spec §6.2).
func (b *Broadcaster) sendable() []*slotEntry {
	var out []*slotEntry
	for _, e := range b.st.entries {
		if e.tx != nil || e.closed || len(e.queued) == 0 {
			continue
		}
		if e.kind == policy.MsgVote && !e.lead && len(e.filling) > 0 && len(e.queued) < b.o.MaxMsgs {
			continue
		}
		out = append(out, e)
	}

	rank := map[policy.MsgKind]int{policy.MsgVeto: 0, policy.MsgRefund: 1, policy.MsgVote: 2}
	slices.SortFunc(out, func(a, c *slotEntry) int {
		return cmp.Or(cmp.Compare(rank[a.kind], rank[c.kind]), cmp.Compare(a.seq, c.seq))
	})

	return out
}

// settleWindows drops unobserved transactions once chain time passes their timeout, and removes the entries of
// windows that chain time has left once nothing in them is pending.
func (b *Broadcaster) settleWindows() {
	s := &b.st
	for k, e := range s.entries {
		if tx := e.tx; tx != nil && tx.phase == sent && !tx.observed && !tx.busy && s.chainTime.After(e.timeout) {
			e.tx, e.closed = nil, true
		}

		if k.window < s.window && e.tx == nil && len(e.queued) == 0 {
			delete(s.entries, k)
		}
	}
}

// applyBuild records a worker's simulation, signing and first broadcast of e's transaction. After Wait reported an
// abandoned build's messages, nothing more is written or deferred for them; what it sent still holds its slot.
func (b *Broadcaster) applyBuild(e *slotEntry, tx *liveTx, res buildResult) {
	tx.busy = false
	if res.seen != nil {
		b.observe(res.seen.Height, res.seen.Time)
	}

	if tx.abandoned {
		b.applyAbandoned(e, tx, res)
		return
	}

	for _, f := range res.failed {
		e.drop(f.planned)
		b.fail(f.planned, f.text)
	}
	for _, d := range res.deferred {
		e.drop(d.planned)
		b.deferMsg(d.planned, d.reason, e.key.window, e.timeout)
	}
	for _, p := range res.released {
		e.drop(p)
	}

	r := res.sent
	if r == nil {
		b.endTx(e)
		return
	}

	tx.msgs, tx.raw, tx.hash, tx.fee = r.msgs, r.raw, r.hash, r.fee

	switch r.outcome {
	case sendSlotTaken:
		b.o.Metrics.RecordTx(resultSlotTaken)
		for _, p := range r.msgs {
			e.drop(p)
			b.deferMsg(p, reasonSlotBusy, e.key.window, e.timeout)
		}
		b.endTx(e)
		e.closed = true
	case sendRejected:
		b.o.Metrics.RecordTx(resultRejected)
		text := fmt.Sprintf("sink transaction rejected: %X code %d: %s", r.hash, r.code, r.log)
		for _, p := range r.msgs {
			e.drop(p)
			b.fail(p, text)
		}
		b.endTx(e)
	default:
		tx.phase = sent
		tx.resend = r.outcome == sendTransport
		tx.observed = b.st.indexed
		if !tx.observed {
			b.leaveFlight(tx)
		}
		for _, p := range r.msgs {
			b.o.Metrics.RecordTxMessage(sdk.MsgTypeURL(p.Msg))
		}
		b.progressed()
	}
}

func (b *Broadcaster) applyAbandoned(e *slotEntry, tx *liveTx, res buildResult) {
	for _, f := range res.failed {
		e.drop(f.planned)
	}
	for _, d := range res.deferred {
		e.drop(d.planned)
	}
	for _, p := range res.released {
		e.drop(p)
	}

	r := res.sent
	switch {
	case r == nil || r.outcome == sendRejected:
		for _, p := range tx.msgs {
			e.drop(p)
		}
		b.endTx(e)
	case r.outcome == sendSlotTaken:
		b.endTx(e)
		e.closed = true
	default:
		tx.msgs, tx.raw, tx.hash, tx.fee = r.msgs, r.raw, r.hash, r.fee
		tx.phase, tx.resend, tx.observed = sent, r.outcome == sendTransport, false
		b.leaveFlight(tx)
	}
}

// applyLookup records the answer of tx(hash) at height h. final means chain time had passed the timeout when the
// lookup started, so a transaction still missing can never be included.
func (b *Broadcaster) applyLookup(e *slotEntry, tx *liveTx, h int64, final bool, r *akash.TxResult, err error) {
	tx.busy = false
	tx.looked = max(tx.looked, h)
	if e.tx != tx || !tx.observed || err != nil {
		return
	}

	hash := fmt.Sprintf("%X", tx.hash)

	switch {
	case r == nil:
		if !final {
			return
		}
		b.o.Metrics.RecordTx(resultExpired)
		for _, p := range tx.msgs {
			b.fail(p, "sink transaction expired: "+hash)
		}
	case r.Code == 0:
		b.o.Metrics.RecordTx(resultIncluded)
		b.o.Metrics.AddTxFees(float64(tx.fee.AmountOf(feeDenom).Int64()))
		for _, p := range tx.msgs {
			b.ev.Emit(events.TxHash{ID: p.ID.String(), TransactionHash: hash})
		}
		if e.kind == policy.MsgRefund {
			b.refundIncluded(r.Height)
		}
		b.progressed()
	case r.Code == replayCode && r.Codespace == replayCodespace && strings.Contains(r.Log, nonceTaken):
		// Another transaction took the slot first; this one paid nothing and changed nothing.
		b.o.Metrics.RecordTx(resultSlotTaken)
		for _, p := range tx.msgs {
			e.drop(p)
			b.deferMsg(p, reasonSlotBusy, e.key.window, e.timeout)
		}
		b.progressed()
	default:
		b.o.Metrics.RecordTx(resultFailed)
		b.o.Metrics.AddTxFees(float64(tx.fee.AmountOf(feeDenom).Int64()))
		text := fmt.Sprintf("sink transaction failed: %s code %d", hash, r.Code)
		for _, p := range tx.msgs {
			b.fail(p, text)
		}
	}

	b.endTx(e)
	e.closed = true
}

// idle reports nothing queued, building, awaiting an observable outcome or deferred, and no follow-up review running
// or waiting to start.
func (b *Broadcaster) idle() bool {
	s := &b.st
	if s.followBusy || (s.refundAt > 0 && !s.refundHeld) || len(s.deferred) > 0 {
		return false
	}

	for _, e := range s.entries {
		if len(e.queued) > 0 || (e.tx != nil && ((e.tx.phase == building && !e.tx.abandoned) || e.tx.observed)) {
			return false
		}
	}

	return true
}

// giveUp ends Wait at its limit (spec §6.5): unresolved transactions are reported unconfirmed and stop being awaited,
// though they keep their slot until expiry; queued, building and deferred messages are reported not sent.
func (b *Broadcaster) giveUp() {
	s := &b.st

	entries := make([]*slotEntry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, c *slotEntry) int { return cmp.Compare(a.seq, c.seq) })

	for _, e := range entries {
		if tx := e.tx; tx != nil && tx.phase == sent && tx.observed {
			b.o.Metrics.RecordTx(resultUnconfirmed)
			for _, p := range tx.msgs {
				b.fail(p, fmt.Sprintf("sink transaction unconfirmed: %X", tx.hash))
			}
			tx.observed = false
			b.leaveFlight(tx)
		}

		if tx := e.tx; tx != nil && tx.phase == building && !tx.abandoned {
			tx.abandoned = true
			for _, p := range tx.msgs {
				b.fail(p, notSent(e))
			}
		}

		for _, p := range e.queued {
			e.drop(p)
			b.fail(p, notSent(e))
		}
		e.clearQueue()
	}

	deferred := make([]*deferral, 0, len(s.deferred))
	for _, d := range s.deferred {
		deferred = append(deferred, d)
	}
	slices.SortFunc(deferred, func(a, c *deferral) int {
		return cmp.Or(sinkid.Compare(a.planned.ID, c.planned.ID), cmp.Compare(a.planned.Key.Kind, c.planned.Key.Kind))
	})

	for _, d := range deferred {
		b.fail(d.planned, fmt.Sprintf("sink message not sent: deferred (%s) until %s", d.reason, rfc3339(d.until)))
	}
	clear(s.deferred)
}

// shutdown releases queued and deferred messages without events (spec §5.7).
func (b *Broadcaster) shutdown() {
	s := &b.st
	s.stopping = true

	for _, e := range s.entries {
		for _, p := range e.queued {
			e.drop(p)
		}
		e.clearQueue()
	}
	clear(s.deferred)
}

// settled reports that no transaction is being built or awaits an observable outcome.
func (b *Broadcaster) settled() bool {
	for _, e := range b.st.entries {
		if e.tx != nil && (e.tx.phase == building || e.tx.observed) {
			return false
		}
	}

	return true
}

// notSent is the report of a message still queued or being built in e when Wait gives up (spec §6.5).
func notSent(e *slotEntry) string { return "sink message not sent: queued until " + rfc3339(e.timeout) }

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
