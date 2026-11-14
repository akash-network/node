// Package scan ports Scanner of contracts/solana/reporter/scan.ts. A pass lists the open transfers and one audit page,
// reviews them in concurrent chunks, each decided from one Akash height and one finalized Solana snapshot, writes the
// review events and hands the planned messages to a Queuer (spec §5.2). It also runs the broadcaster's follow-up
// reviews (spec §6.6).
package scan

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/metrics"
	"pkg.akt.dev/node/v2/operator/reporter/observe"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	// chunkSize is how many transfers share one snapshot: with the four deployment accounts, the Solana RPC's limit
	// of 100 keys per getMultipleAccounts call.
	chunkSize            = 96
	defaultRetryInterval = time.Second

	alertChanged  = "light-client-changed"
	alertMismatch = "light-client-mismatch"
	alertAudit    = "audit-unavailable"

	index         = "canonical"
	modeBroadcast = "broadcast"
	modePreview   = "preview"
	kindAbsent    = "absent"
	kindResolved  = "resolved"

	reviewOK    = "ok"
	reviewError = "error"
)

var errNotReporter = errors.New("not a current reporter")

// ErrPassAborted is what Review and ReviewRefundHead return, at once and without writing or counting anything, after a
// pass ended on a light-client pin mismatch and until the next pass begins. The pass's own alert and error stay its
// last output (spec §5.6).
var ErrPassAborted = errors.New("follow-up review skipped: the pass ended on a light-client pin mismatch")

// Queuer takes the messages a review planned. Queue reports whether the message was taken; false means it was skipped
// or deferred.
type Queuer interface {
	Queue(policy.Planned) bool
}

type queuerKey struct{}

// WithQueuer returns a context under which the reviews of a broadcasting scanner hand their plans to q instead of
// the scanner's Queuer, so that the broadcaster can tell which review filled a batch (spec §6.2).
func WithQueuer(ctx context.Context, q Queuer) context.Context {
	return context.WithValue(ctx, queuerKey{}, q)
}

// Options configures a Scanner.
type Options struct {
	Reporter string
	// Broadcast marks review events "broadcast" instead of "preview".
	Broadcast    bool
	Workers      int
	PageSize     int
	AuditPerScan int
	// Clock times the follow-up reviews' height retries; nil means the system clock.
	Clock clock.Clock
	// RetryInterval is how long a follow-up review waits before asking again for a high enough snapshot; zero means
	// one second, the tracker's refresh rate.
	RetryInterval time.Duration
	Metrics       *metrics.Metrics
}

// Scanner runs passes and follow-up reviews. Pass must not run concurrently with itself; Review and ReviewRefundHead
// may run alongside a pass.
type Scanner struct {
	akash    *akash.Client
	observer *observe.Observer
	events   *events.Writer
	opts     Options
	queue    Queuer

	// cursor is where the next audit page starts.
	cursor *akash.AuditCursor
	// followUpFailures counts the failures of follow-up reviews until a pass adds them to its own.
	followUpFailures atomic.Int64
	// aborted is set from a pin mismatch that ended a pass until the next pass begins; follow-ups then skip.
	aborted atomic.Bool
}

// New returns a scanner. q is nil in preview.
func New(a *akash.Client, o *observe.Observer, ev *events.Writer, opts Options, q Queuer) *Scanner {
	opts.Workers = max(opts.Workers, 1)
	if opts.Clock == nil {
		opts.Clock = clock.Real()
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = defaultRetryInterval
	}

	return &Scanner{akash: a, observer: o, events: ev, opts: opts, queue: q}
}

// entry is a transfer to review; audit marks one found only by the audit page.
type entry struct {
	id    sinkid.ID
	audit bool
}

// tally counts a pass's failures and reviews; chunks update it concurrently.
type tally struct {
	failures atomic.Int64
	reviewed atomic.Int64
}

// reviewRun is the context of one chunk's reviews. stop ends them between transfers, as TS checks its abort signal;
// requests run under req, which a pin mismatch cancels but shutdown does not, so in-flight requests finish or time out
// (spec §5.7).
type reviewRun struct {
	stop  context.Context
	req   context.Context
	state *akash.State
	tally *tally
	// followUp skips transfers that are no longer open.
	followUp bool
}

// OnChange writes the light-client-changed alert. The observer calls it under its follow lock, before any review
// decided under the new client; it must not call back into the observer.
func (s *Scanner) OnChange(c observe.Change) {
	s.events.Emit(events.LightClientChanged{
		Alert:    alertChanged,
		Previous: c.Previous.String(),
		Current:  c.Current.String(),
		Slot:     events.U64(c.ContextSlot),
	})
	s.opts.Metrics.RecordAlert(alertChanged)
}

// Pass runs one pass and returns its failures. ctx bounds the reviews: once it ends, no new chunk or transfer review
// starts. Once the reviews end, settle, when set, runs once and its failures count; then the summary is written if the
// pass reached its reviews, or else the pass error.
func (s *Scanner) Pass(ctx context.Context, settle func() int) int {
	began := time.Now()
	s.aborted.Store(false)

	var t tally
	summary, err := s.pass(ctx, &t)

	var mismatch *observe.MismatchError
	if errors.As(err, &mismatch) {
		s.aborted.Store(true)
	}

	if settle != nil {
		t.failures.Add(int64(settle()))
	}
	// Follow-up reviews count in the pass they finish in; in scan, those settle waited for (spec �6.5).
	t.failures.Add(s.followUpFailures.Swap(0))

	if err != nil {
		t.failures.Add(1)
		s.passError(err)
	} else {
		summary.Failures = int(t.failures.Load())
		s.events.Emit(summary)
	}

	s.opts.Metrics.ObservePass(time.Since(began), int(t.reviewed.Load()))

	return int(t.failures.Load())
}

// pass starts, lists, merges and reviews, returning the summary without failures. An error ends the pass without one.
// ctx stops only the reviews: the start and listing requests finish or time out (spec §5.7), so a cancelled pass still
// reports the transfers it listed.
func (s *Scanner) pass(ctx context.Context, t *tally) (*events.Summary, error) {
	req := context.WithoutCancel(ctx)

	deployment, err := s.observer.Deployment(req)
	if err != nil {
		return nil, err
	}

	st, err := s.akash.Latest(req)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(st.Policy.Members, s.opts.Reporter) {
		return nil, errNotReporter
	}

	open, audited, err := s.list(req, st.Height, t)
	if err != nil {
		return nil, err
	}

	if err := s.reviewAll(ctx, merge(open, audited), t); err != nil {
		return nil, err
	}

	return &events.Summary{
		Scanned:            len(open),
		Audited:            len(audited),
		Index:              index,
		LightClientProgram: deployment.LightClient.String(),
	}, nil
}

// list reads the open transfers and one audit page at height, concurrently. A listing error ends the pass. An audit
// error writes audit-unavailable, counts one failure and keeps the cursor; a last page drops it, as TS does.
func (s *Scanner) list(ctx context.Context, height int64, t *tally) (open, audited []sinkid.ID, err error) {
	var (
		wg       sync.WaitGroup
		page     akash.AuditPage
		auditErr error
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		page, auditErr = s.akash.Audit(ctx, height, s.cursor, min(s.opts.AuditPerScan, s.opts.PageSize))
	}()

	open, err = s.akash.ListOpen(ctx, height, s.opts.PageSize)
	wg.Wait()

	if err != nil {
		return nil, nil, err
	}

	if auditErr != nil {
		t.failures.Add(1)
		s.events.Emit(events.AuditUnavailable{Alert: alertAudit, Error: auditErr.Error()})
		s.opts.Metrics.RecordAlert(alertAudit)

		return open, nil, nil
	}

	s.cursor = nil
	if page.HasMore {
		s.cursor = page.Cursor
	}

	return open, page.IDs, nil
}

// merge keeps the open transfers in order, then the audit entries not already listed.
func merge(open, audited []sinkid.ID) []entry {
	out := make([]entry, 0, len(open)+len(audited))
	seen := make(map[string]struct{}, len(open))

	for _, id := range open {
		seen[string(id.KeySuffix())] = struct{}{}
		out = append(out, entry{id: id})
	}

	for _, id := range audited {
		if _, ok := seen[string(id.KeySuffix())]; !ok {
			out = append(out, entry{id: id, audit: true})
		}
	}

	return out
}

// reviewAll feeds chunks to the workers until every chunk is reviewed, ctx ends or a light-client pin mismatch
// stops the pass. A mismatch cancels the running chunks' requests and is returned once they have stopped.
func (s *Scanner) reviewAll(ctx context.Context, entries []entry, t *tally) error {
	req, abort := context.WithCancelCause(context.WithoutCancel(ctx))
	defer abort(nil)

	chunks := make(chan []entry)

	var wg sync.WaitGroup
	for range s.opts.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for chunk := range chunks {
				if err := s.reviewChunk(ctx, req, chunk, t); err != nil {
					abort(err)
				}
			}
		}()
	}

feed:
	for start := 0; start < len(entries); start += chunkSize {
		if ctx.Err() != nil || req.Err() != nil {
			break
		}

		select {
		case chunks <- entries[start:min(start+chunkSize, len(entries))]:
		case <-ctx.Done():
			break feed
		case <-req.Done():
			break feed
		}
	}

	close(chunks)
	wg.Wait()

	if req.Err() != nil {
		return context.Cause(req)
	}

	return nil
}

// reviewChunk reviews one chunk of a pass from the tracker's latest snapshot. It returns only a pin mismatch.
func (s *Scanner) reviewChunk(stop, req context.Context, chunk []entry, t *tally) error {
	if stop.Err() != nil || req.Err() != nil {
		return nil
	}

	r := reviewRun{stop: stop, req: req, tally: t}

	st, err := s.akash.Latest(req)
	if err != nil {
		for _, e := range chunk {
			s.fail(r, e.id, err)
		}

		return nil
	}
	r.state = st

	return s.reviewAt(r, chunk)
}

// reviewAt re-reads the chunk's transfers at the snapshot height, takes one Solana snapshot of their status accounts
// and reviews them in order. Errors fail single transfers; only a pin mismatch is returned.
func (s *Scanner) reviewAt(r reviewRun, chunk []entry) error {
	transfers := make([]sinkv1.TransferState, len(chunk))
	errs := make([]error, len(chunk))

	var wg sync.WaitGroup
	for i, e := range chunk {
		wg.Add(1)
		go func() {
			defer wg.Done()
			transfers[i], errs[i] = s.akash.Transfer(r.req, r.state.Height, e.id)
		}()
	}
	wg.Wait()

	skip := make([]bool, len(chunk))
	ids := make([]sinkid.ID, 0, len(chunk))
	for i, e := range chunk {
		if errs[i] != nil {
			continue
		}
		if r.followUp && !isOpen(transfers[i]) {
			skip[i] = true
			continue
		}
		ids = append(ids, e.id)
	}

	var (
		snapshot *observe.Chunk
		chunkErr error
	)
	if len(ids) > 0 {
		snapshot, chunkErr = s.observer.Chunk(r.req, ids)

		var mismatch *observe.MismatchError
		if errors.As(chunkErr, &mismatch) {
			return chunkErr
		}
	}

	for i, e := range chunk {
		if r.stop.Err() != nil || r.req.Err() != nil {
			return nil
		}
		if skip[i] {
			continue
		}

		err := errs[i]
		if err == nil {
			err = chunkErr
		}
		if err == nil {
			err = s.review(r, snapshot, transfers[i], e.audit)
		}
		if err != nil {
			s.fail(r, e.id, err)
		}
	}

	return nil
}

// review decides one transfer, writes its event and then queues its messages.
func (s *Scanner) review(r reviewRun, chunk *observe.Chunk, t sinkv1.TransferState, audit bool) error {
	rec, err := sinkid.DecodeRecord(t)
	if err != nil {
		return err
	}
	id := rec.ID.String()

	if audit && !pending(t.Status) {
		f, err := s.observer.Finalized(chunk, rec)
		if err != nil {
			return err
		}

		if f.Kind == policy.KindResolved && policy.SettledConsistently(t, f.Outcome) {
			s.events.Emit(events.Review{ID: id, Audit: true, Observed: kindResolved})
			s.reviewed(r, reviewOK)

			return nil
		}
	}

	observed, err := s.observer.Observe(r.req, chunk, rec, policy.ReportedObservations(t), s.ownVote(t))
	if err != nil {
		return err
	}

	plan, err := policy.PlanOutcome(s.opts.Reporter, policy.Snapshot{
		Params:      r.state.Params,
		Policy:      r.state.Policy,
		RefundUsage: r.state.RefundUsage,
		Epoch:       r.state.Epoch,
		Time:        r.state.Time.Unix(),
		RefundHead:  r.state.RefundHead,
		Transfer:    t,
	}, observed)
	if err != nil {
		return err
	}

	messages := make([]events.Message, len(plan.Messages))
	for i, msg := range plan.Messages {
		if messages[i], err = events.MessageOf(msg); err != nil {
			return err
		}
	}

	s.events.Emit(events.Review{
		ID:       id,
		Mode:     s.mode(),
		Audit:    audit,
		Observed: observedEvent(observed),
		Messages: messages,
		Alerts:   plan.Alerts,
	})
	s.reviewed(r, reviewOK)
	for _, alert := range plan.Alerts {
		s.opts.Metrics.RecordAlert(alert)
	}

	// As in TS, nothing is handed on once the reviews were told to stop.
	q := s.queue
	if q != nil {
		if own, ok := r.stop.Value(queuerKey{}).(Queuer); ok {
			q = own
		}
	}
	if q == nil || r.stop.Err() != nil {
		return nil
	}

	for _, msg := range plan.Messages {
		q.Queue(policy.Planned{
			ID:         rec.ID,
			Msg:        msg,
			Key:        policy.KeyOf(rec.ID, msg),
			PlanHeight: r.state.Height,
			PlanTime:   r.state.Time,
		})
	}

	return nil
}

// fail writes a transfer's error, unless a pin mismatch cancelled its requests: the pass then stops without further
// events. A Solana evidence error is the transfer's own, even when it wraps a request timeout.
func (s *Scanner) fail(r reviewRun, id sinkid.ID, err error) {
	evidence := errors.Is(err, aktsolana.ErrBlockMissing) ||
		errors.Is(err, aktsolana.ErrNotInBlock) ||
		errors.Is(err, aktsolana.ErrMultipleInBlock)
	if !evidence && r.req.Err() != nil {
		return
	}

	r.tally.failures.Add(1)
	s.events.Emit(events.TransferError{ID: id.String(), Error: err.Error()})
	s.reviewed(r, reviewError)
}

func (s *Scanner) reviewed(r reviewRun, result string) {
	r.tally.reviewed.Add(1)
	s.opts.Metrics.RecordReview(result)
}

// passError writes the error that ended a pass, after the light-client-mismatch alert when it is a pin mismatch.
func (s *Scanner) passError(err error) {
	var mismatch *observe.MismatchError
	if errors.As(err, &mismatch) {
		s.mismatchAlert(mismatch)
	}

	s.events.Emit(events.PassError{Error: err.Error()})
}

func (s *Scanner) mismatchAlert(m *observe.MismatchError) {
	s.events.Emit(events.LightClientMismatch{Alert: alertMismatch, Pinned: m.Pinned.String(), Current: m.Current.String()})
	s.opts.Metrics.RecordAlert(alertMismatch)
}

func (s *Scanner) mode() string {
	if s.opts.Broadcast {
		return modeBroadcast
	}

	return modePreview
}

// ownVote returns this reporter's stored vote observation, if any.
func (s *Scanner) ownVote(t sinkv1.TransferState) *sinkv1.Observation {
	for _, vote := range t.Settlement.Votes {
		if vote.Reporter == s.opts.Reporter {
			observation := vote.Observation
			return &observation
		}
	}

	return nil
}

// ReviewRefundHead reviews the refund head of a snapshot at least minHeight high as a one-transfer chunk. A snapshot
// without a refund head reviews nothing. After a pass that ended on a pin mismatch, it returns ErrPassAborted.
func (s *Scanner) ReviewRefundHead(ctx context.Context, minHeight int64) error {
	if s.aborted.Load() {
		return ErrPassAborted
	}

	st, err := s.latestAt(ctx, minHeight)
	if err != nil {
		return err
	}
	if st.RefundHead == nil {
		return nil
	}
	if s.aborted.Load() {
		return ErrPassAborted
	}

	return s.followUp(ctx, st, []entry{{id: *st.RefundHead}})
}

// Review reviews ids in chunks of up to 96, each from a snapshot at least minHeight high, writing the normal review
// events and queuing what it plans. IDs that are no longer open are skipped. After a pass that ended on a pin
// mismatch, it returns ErrPassAborted.
func (s *Scanner) Review(ctx context.Context, ids []sinkid.ID, minHeight int64) error {
	entries := make([]entry, len(ids))
	for i, id := range ids {
		entries[i] = entry{id: id}
	}

	for start := 0; start < len(entries); start += chunkSize {
		if s.aborted.Load() {
			return ErrPassAborted
		}

		st, err := s.latestAt(ctx, minHeight)
		if err != nil {
			return err
		}
		if s.aborted.Load() {
			return ErrPassAborted
		}

		if err := s.followUp(ctx, st, entries[start:min(start+chunkSize, len(entries))]); err != nil {
			return err
		}
	}

	return nil
}

// followUp reviews one chunk of a follow-up from st. Its failures, and a pin mismatch, count in the next pass's
// failures. A pin mismatch writes its alert and is returned; a ctx that ended during the reviews returns its error.
func (s *Scanner) followUp(ctx context.Context, st *akash.State, chunk []entry) error {
	var t tally
	err := s.reviewAt(reviewRun{stop: ctx, req: context.WithoutCancel(ctx), state: st, tally: &t, followUp: true}, chunk)
	s.followUpFailures.Add(t.failures.Load())

	var mismatch *observe.MismatchError
	if errors.As(err, &mismatch) {
		// A pass that ended on the mismatch meanwhile writes the one alert.
		if s.aborted.Load() {
			return ErrPassAborted
		}

		s.followUpFailures.Add(1)
		s.mismatchAlert(mismatch)
		return err
	}

	return ctx.Err()
}

// latestAt returns the tracker's latest snapshot once it is at least minHeight high. A failed snapshot is retried like
// a low one, every RetryInterval, so progression cannot stop on a transient node error (spec §6.6). When ctx ends, it
// returns the last snapshot error, or ctx's error when the last snapshot was only too low.
func (s *Scanner) latestAt(ctx context.Context, minHeight int64) (*akash.State, error) {
	var last error

	for {
		st, err := s.akash.Latest(ctx)
		switch {
		case err == nil && st.Height >= minHeight:
			return st, nil
		case err == nil:
			last = nil
		case ctx.Err() == nil:
			last = err
		}

		select {
		case <-ctx.Done():
			if last != nil {
				return nil, last
			}

			return nil, ctx.Err()
		case <-s.opts.Clock.After(s.opts.RetryInterval):
		}
	}
}

// pending reports the statuses whose audit entries are not checked for the early exit.
func pending(st sinkv1.Status) bool {
	return st == sinkv1.StatusTransferPending || st == sinkv1.StatusVoided || st == sinkv1.StatusRefundPending
}

// isOpen reports a transfer a pass would list: held, or in one of the pending buckets.
func isOpen(t sinkv1.TransferState) bool {
	return t.Settlement.Hold != sinkv1.HoldNone || pending(t.Status)
}

func observedEvent(o policy.Observed) any {
	if o.Kind == policy.KindResolved {
		return events.Resolved{
			Kind:        kindResolved,
			ContextSlot: events.U64(o.ContextSlot),
			Observation: events.ObservationOf(o.Observation),
		}
	}

	var limit uint64
	if o.SlotLimit != nil {
		limit = *o.SlotLimit
	}

	return events.Absent{Kind: kindAbsent, ContextSlot: events.U64(o.ContextSlot), SlotLimit: events.U64(limit)}
}
