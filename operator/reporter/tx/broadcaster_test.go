package tx

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gagliardetto/solana-go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/observe"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/scan"
	"pkg.akt.dev/node/v2/operator/reporter/signer"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

// start is 30 s into a two-minute window; the scenario's latest block is a second older.
var start = time.Date(2026, 11, 12, 10, 0, 30, 0, time.UTC)

// broadcast-mix plans a vote for transfers 0-39, a veto for 40-43 and a refund for the head, 44.
const (
	mixVotes   = 40
	firstVeto  = 40
	headRefund = 44

	testMaxAge     = 10 * time.Minute
	testAccountNum = 7
	slotLead       = 600
)

var testPrices = sdk.NewDecCoins(sdk.NewDecCoinFromDec("uakt", sdkmath.LegacyMustNewDecFromStr("0.025")))

// output collects event lines; it is safe to read while a writer runs.
type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.buf.Write(p)
}

func (o *output) text() string {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.buf.String()
}

// setup configures a harness.
type setup struct {
	// scenario names the scenario the node serves; empty means broadcast-mix.
	scenario string
	// change edits the scenario before the node starts.
	change func(*simnode.Scenario)
	// real runs on the system clock, with a block every blockInterval.
	real          bool
	blockInterval time.Duration
	// minRefresh is the clients' tracker refresh interval on the system clock; zero means 5 ms.
	minRefresh time.Duration
}

// harness is one simnode serving a named scenario, broadcast-mix by default, on a fake clock unless the setup asks
// for the real one.
type harness struct {
	t     *testing.T
	node  *simnode.Node
	clock *clock.Fake
	clk   clock.Clock
	chain *akash.Client
	ids   []sinkid.ID
	// lead is a new block's time minus the clock.
	lead   time.Duration
	signer signer.Signer
	// step is how far tick advances the fake clock: the broadcasters' refresh interval.
	step       time.Duration
	minRefresh time.Duration
}

func newHarness(t *testing.T, s setup) *harness {
	t.Helper()

	name := s.scenario
	if name == "" {
		name = "broadcast-mix"
	}
	sc := simnode.Scenarios()[name]()
	if s.change != nil {
		s.change(&sc)
	}

	h := &harness{t: t, step: time.Second, minRefresh: s.minRefresh}
	opts := simnode.Options{BlockInterval: s.blockInterval}
	if s.real {
		h.clk = clock.Real()
	} else {
		h.clock = clock.NewFake(start)
		h.clk = h.clock
	}
	opts.Clock = h.clk

	for _, tr := range sc.Akash.Transfers {
		id, err := sinkid.FromProto(tr.Transfer.ID)
		require.NoError(t, err)
		h.ids = append(h.ids, id)
	}

	n, err := simnode.Start(t.Context(), sc, opts)
	require.NoError(t, err)
	t.Cleanup(n.Close)
	h.node = n

	h.chain = h.dial()
	before := h.clk.Now()
	st := h.snapshot()
	h.lead = st.Time.Sub(before)
	h.signer = reporterSigner(t)

	return h
}

// dial connects a new client, which reads the node's transaction index setting now.
func (h *harness) dial() *akash.Client {
	h.t.Helper()

	refresh := time.Nanosecond
	if h.clock == nil {
		refresh = cmp.Or(h.minRefresh, 5*time.Millisecond)
	}

	a, err := akash.Dial(h.t.Context(), akash.Options{
		Node:           h.node.CometURL,
		GRPCAddr:       h.node.GRPCAddr,
		ChainID:        simnode.ChainID,
		MaxAge:         testMaxAge,
		RequestTimeout: 5 * time.Second,
		Concurrency:    16,
		MinRefresh:     refresh,
		Clock:          h.clk,
	})
	require.NoError(h.t, err)
	h.t.Cleanup(func() { _ = a.Close() })

	return a
}

func reporterSigner(t *testing.T) signer.Signer {
	t.Helper()

	reg := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(reg)
	kr := keyring.NewInMemory(codec.NewProtoCodec(reg))

	rec, err := kr.NewAccount("reporter", simnode.ReporterMnemonic, "", hd.CreateHDPath(sdk.CoinType, 0, 0).String(), hd.Secp256k1)
	require.NoError(t, err)
	addr, err := rec.GetAddress()
	require.NoError(t, err)
	require.Equal(t, simnode.Reporter, addr.String())

	s, err := signer.NewKeyring(kr, "reporter", addr)
	require.NoError(t, err)

	return s
}

func (h *harness) builder(s signer.Signer) *Builder {
	return NewBuilder(simnode.TxConfig(), simnode.ChainID, testAccountNum, s)
}

func (h *harness) snapshot() *akash.State {
	h.t.Helper()

	st, err := h.chain.Latest(h.t.Context())
	require.NoError(h.t, err)

	return st
}

// tick advances the fake clock by one refresh interval and gives the broadcaster time to react.
func (h *harness) tick() {
	h.clock.Advance(h.step)
	time.Sleep(10 * time.Millisecond)
}

// eventually ticks until cond holds, failing after 60 ticks.
func (h *harness) eventually(cond func() bool, msg string) {
	h.t.Helper()

	for range 60 {
		if cond() {
			return
		}
		h.tick()
	}
	require.True(h.t, cond(), msg)
}

// blockAt commits a block whose time is t, which must lie ahead of the clock's block time.
func (h *harness) blockAt(t time.Time) {
	h.t.Helper()

	d := t.Sub(h.clock.Now().Add(h.lead))
	require.Positive(h.t, d)
	h.clock.Advance(d)
	h.node.CommitBlock()
}

// commit commits a block and lets the broadcaster see it.
func (h *harness) commit() {
	h.node.CommitBlock()
	h.tick()
	h.tick()
}

// plan is the message broadcast-mix plans for transfer i, from st.
func (h *harness) plan(st *akash.State, i int) policy.Planned {
	id := h.ids[i]

	var msg sdk.Msg
	switch {
	case i < mixVotes:
		msg = voteFor(id, sinkv1.OutcomeVoided, 1)
	case i < headRefund:
		msg = &sinkv1.MsgVetoVoid{Signer: simnode.Reporter, ID: id.Proto(), Reason: "No Solana status"}
	default:
		msg = &sinkv1.MsgFinalizeRefund{Executor: simnode.Reporter, ID: id.Proto()}
	}

	return plannedAt(st, id, msg)
}

func voteFor(id sinkid.ID, outcome sinkv1.Outcome, round uint64) *sinkv1.MsgReportOutcome {
	return &sinkv1.MsgReportOutcome{
		Reporter:      simnode.Reporter,
		ID:            id.Proto(),
		PolicyVersion: 1,
		Round:         round,
		Observation:   sinkv1.Observation{Outcome: outcome, Slot: 900, TransactionSignature: bytes.Repeat([]byte{1}, 64)},
	}
}

func plannedAt(st *akash.State, id sinkid.ID, msg sdk.Msg) policy.Planned {
	return policy.Planned{ID: id, Msg: msg, Key: policy.KeyOf(id, msg), PlanHeight: st.Height, PlanTime: st.Time}
}

func slotOf(p policy.Planned) int { return SlotOf(p.ID, p.Key) }

// sameSlotVotes returns n valid votes of broadcast-mix, planned from st, that share one slot.
func (h *harness) sameSlotVotes(st *akash.State, n int) []policy.Planned {
	h.t.Helper()

	bySlot := map[int][]policy.Planned{}
	for _, outcome := range []sinkv1.Outcome{sinkv1.OutcomeVoided, sinkv1.OutcomeMinted} {
		for i := range mixVotes {
			p := plannedAt(st, h.ids[i], voteFor(h.ids[i], outcome, 1))
			s := slotOf(p)
			if len(bySlot[s]) < n && !containsID(bySlot[s], p.ID) {
				bySlot[s] = append(bySlot[s], p)
			}
			if len(bySlot[s]) == n {
				return bySlot[s]
			}
		}
	}

	h.t.Fatalf("broadcast-mix has no %d votes of distinct transfers in one slot", n)

	return nil
}

func containsID(ps []policy.Planned, id sinkid.ID) bool {
	for _, p := range ps {
		if sinkid.Compare(p.ID, id) == 0 {
			return true
		}
	}

	return false
}

// sameSlot returns n plans of p's kind with keys other than p's in p's slot: votes of later rounds, or vetoes and
// refunds of other transfers. They need not be valid on chain.
func sameSlot(t *testing.T, p policy.Planned, n int) []policy.Planned {
	t.Helper()

	var out []policy.Planned
	for k := uint64(2); len(out) < n && k < 1_000_000; k++ {
		var q policy.Planned
		switch m := p.Msg.(type) {
		case *sinkv1.MsgReportOutcome:
			v := *m
			v.Round = k
			q = policy.Planned{ID: p.ID, Msg: &v}
		case *sinkv1.MsgVetoVoid:
			id := p.ID
			id.Sequence = k
			v := *m
			v.ID = id.Proto()
			q = policy.Planned{ID: id, Msg: &v}
		case *sinkv1.MsgFinalizeRefund:
			id := p.ID
			id.Sequence = k
			v := *m
			v.ID = id.Proto()
			q = policy.Planned{ID: id, Msg: &v}
		}
		q.Key, q.PlanHeight, q.PlanTime = policy.KeyOf(q.ID, q.Msg), p.PlanHeight, p.PlanTime

		if slotOf(q) == slotOf(p) {
			out = append(out, q)
		}
	}
	require.Len(t, out, n)

	return out
}

// decoded is a broadcast transaction.
type decoded struct {
	raw     []byte
	msgs    []sdk.Msg
	ids     []string
	timeout time.Time
	gas     uint64
	fee     sdk.Coins
}

func (d decoded) hash() string {
	sum := sha256.Sum256(d.raw)
	return fmt.Sprintf("%X", sum[:])
}

func decodeTx(t *testing.T, raw []byte) decoded {
	t.Helper()

	tx, err := simnode.TxConfig().TxDecoder()(raw)
	require.NoError(t, err)

	u, ok := tx.(sdk.TxWithUnordered)
	require.True(t, ok)
	require.True(t, u.GetUnordered())
	f, ok := tx.(sdk.FeeTx)
	require.True(t, ok)

	d := decoded{raw: raw, msgs: tx.GetMsgs(), timeout: u.GetTimeoutTimeStamp(), gas: f.GetGas(), fee: f.GetFee()}
	for _, msg := range d.msgs {
		d.ids = append(d.ids, msgID(t, msg))
	}

	return d
}

func msgID(t *testing.T, msg sdk.Msg) string {
	t.Helper()

	var id sinkv1.ID
	switch m := msg.(type) {
	case *sinkv1.MsgReportOutcome:
		id = m.ID
	case *sinkv1.MsgVetoVoid:
		id = m.ID
	case *sinkv1.MsgFinalizeRefund:
		id = m.ID
	default:
		t.Fatalf("unexpected message %T", msg)
	}

	sid, err := sinkid.FromProto(id)
	require.NoError(t, err)

	return sid.String()
}

func (h *harness) broadcasts() []decoded {
	raws := h.node.Broadcasts()
	out := make([]decoded, 0, len(raws))
	for _, raw := range raws {
		out = append(out, decodeTx(h.t, raw))
	}

	return out
}

func (h *harness) waitBroadcasts(n int) []decoded {
	h.t.Helper()

	require.Eventually(h.t, func() bool { return len(h.node.Broadcasts()) >= n }, 10*time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	got := h.broadcasts()
	require.Len(h.t, got, n)

	return got
}

// executions returns how often each message key of the node executed, and checks that none executed twice.
func (h *harness) executions() map[string]int {
	h.t.Helper()

	got := h.node.Executions()
	for key, n := range got {
		require.LessOrEqual(h.t, n, 1, "%s executed %d times", key, n)
	}

	return got
}

// executedVote is the execution key of id's round-1 voided vote.
func executedVote(id sinkid.ID) string {
	return fmt.Sprintf("%s|%s|%d|1|1", sdk.MsgTypeURL(&sinkv1.MsgReportOutcome{}), id, sinkv1.OutcomeVoided)
}

func executedVeto(id sinkid.ID, round uint64) string {
	return fmt.Sprintf("%s|%s|%d", sdk.MsgTypeURL(&sinkv1.MsgVetoVoid{}), id, round)
}

// gatedSigner wraps a signer: gate, when set, holds every signature until it is closed; fail fails every signature;
// limits declares a sign-bytes limit per call, the last repeating.
type gatedSigner struct {
	signer.Signer
	gate   chan struct{}
	fail   atomic.Pointer[string]
	signs  atomic.Int64
	mu     sync.Mutex
	limits []int
}

func (s *gatedSigner) SignBytesLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.limits) == 0 {
		return 0
	}

	limit := s.limits[0]
	if len(s.limits) > 1 {
		s.limits = s.limits[1:]
	}

	return limit
}

func (s *gatedSigner) Sign(ctx context.Context, signBytes []byte) ([]byte, error) {
	s.signs.Add(1)

	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if text := s.fail.Load(); text != nil {
		return nil, fmt.Errorf("%s", *text)
	}

	return s.Signer.Sign(ctx, signBytes)
}

// proc is one reporter process: a broadcaster, its event output and, once wired, its scanner.
type proc struct {
	h       *harness
	b       *Broadcaster
	o       Options
	chain   *akash.Client
	out     *output
	ev      *events.Writer
	scanner *scan.Scanner
	cancel  context.CancelFunc
	done    chan struct{}

	syncs, consumed int
}

func (h *harness) options() Options {
	return Options{
		Reporter:      simnode.Reporter,
		MaxMsgs:       25,
		MaxInFlight:   8,
		GasAdjustment: 1.3,
		GasPrices:     testPrices,
		Clock:         h.clk,
	}
}

// start runs a broadcaster on chain (h.chain when nil) until the test ends; wrap, when given, changes what the
// broadcaster sees of the chain.
func (h *harness) start(o Options, s signer.Signer, chain *akash.Client, wrap ...func(Chain) Chain) *proc {
	return h.startCtx(context.Background(), o, s, chain, wrap...)
}

func (h *harness) startCtx(ctx context.Context, o Options, s signer.Signer, chain *akash.Client, wrap ...func(Chain) Chain) *proc {
	h.t.Helper()

	if chain == nil {
		chain = h.chain
	}

	p := &proc{h: h, o: o, chain: chain, out: &output{}, done: make(chan struct{})}
	p.ev = events.NewWriter(p.out)
	var c Chain = chain
	for _, w := range wrap {
		c = w(c)
	}
	p.b = New(o, c, h.builder(s), p.ev)

	ctx, p.cancel = context.WithCancel(ctx)
	go func() {
		defer close(p.done)
		_ = p.b.Run(ctx)
	}()

	h.t.Cleanup(func() {
		p.stop()
		_ = p.ev.Close()
	})

	return p
}

// stop cancels Run and waits for it, advancing a fake clock through the shutdown grace.
func (p *proc) stop() {
	p.cancel()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-p.done:
			return
		case <-time.After(5 * time.Millisecond):
		}
		if p.h.clock != nil {
			p.h.clock.Advance(time.Second)
		}
	}
	p.h.t.Error("Run did not return after its context ended")
}

// scanner builds a scanner over chain that writes to ev and plans into q.
func (h *harness) scanner(chain *akash.Client, ev *events.Writer, q scan.Queuer, retry time.Duration) *scan.Scanner {
	return h.pinnedScanner(chain, ev, q, retry, nil)
}

// pinnedScanner is scanner with a light-client pin; nil follows the bridge.
func (h *harness) pinnedScanner(chain *akash.Client, ev *events.Writer, q scan.Queuer, retry time.Duration, pin *solana.PublicKey) *scan.Scanner {
	rpc := aktsolana.NewRPC(aktsolana.RPCOptions{URL: h.node.SolanaURL, Timeout: 5 * time.Second, Concurrency: 8, BlockConcurrency: 2})

	var s *scan.Scanner
	o := observe.New(rpc, aktsolana.NewEvidenceIndex(rpc, simnode.ProgramID, 256, 64<<20), observe.Options{
		GenesisHash:    simnode.GenesisHash,
		ProgramID:      simnode.ProgramID,
		Mint:           simnode.MintAddress,
		LightClientPin: pin,
		ChainID:        simnode.ChainID,
		MaxAge:         testMaxAge,
		SlotLead:       slotLead,
		Clock:          h.clk,
	}, func(c observe.Change) { s.OnChange(c) })

	s = scan.New(chain, o, ev, scan.Options{
		Reporter:      simnode.Reporter,
		Broadcast:     true,
		Workers:       4,
		PageSize:      500,
		AuditPerScan:  10,
		Clock:         h.clk,
		RetryInterval: retry,
	}, q)

	return s
}

// wire builds the scanner that plans into p's broadcaster and reviews for it, through reviewer when set.
func (p *proc) wire(reviewer func(*scan.Scanner) Reviewer) *scan.Scanner {
	s := p.h.scanner(p.chain, p.ev, p.b, p.b.o.RefreshInterval)

	var r Reviewer = s
	if reviewer != nil {
		r = reviewer(s)
	}
	p.b.SetReviewer(r)
	p.scanner = s

	return s
}

// lines returns the event lines written since the previous call.
func (p *proc) lines() []string {
	t := p.h.t
	t.Helper()

	p.syncs++
	marker := `{"sync":` + strconv.Itoa(p.syncs) + `}`
	p.ev.Emit(json.RawMessage(marker))

	var all []string
	require.Eventually(t, func() bool {
		all = strings.Split(strings.TrimSuffix(p.out.text(), "\n"), "\n")
		return all[len(all)-1] == marker
	}, 5*time.Second, time.Millisecond)

	var out []string
	for _, l := range all[p.consumed:] {
		if !strings.HasPrefix(l, `{"sync":`) {
			out = append(out, l)
		}
	}
	p.consumed = len(all)

	return out
}

// count returns how many events p wrote that lines has not returned yet.
func (p *proc) count() int {
	n := 0
	for _, l := range strings.Split(strings.TrimSuffix(p.out.text(), "\n"), "\n")[p.consumed:] {
		if l != "" && !strings.HasPrefix(l, `{"sync":`) {
			n++
		}
	}

	return n
}

// outcomes returns the transactionHash and error events among lines by ID.
func outcomes(t *testing.T, lines []string) (hashes, errs map[string]string) {
	t.Helper()

	hashes, errs = map[string]string{}, map[string]string{}
	for _, l := range lines {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m), l)

		id, _ := m["id"].(string)
		if h, ok := m["transactionHash"].(string); ok {
			_, dup := hashes[id]
			require.False(t, dup, "a second transactionHash for %s", id)
			hashes[id] = h
		}
		if e, ok := m["error"].(string); ok && id != "" {
			errs[id] = e
		}
	}

	return hashes, errs
}

// state reads the owner's state through f.
func (p *proc) state(f func(*ownerState)) {
	require.True(p.h.t, p.b.inspect(f), "the broadcaster has stopped")
}

func (p *proc) deferred() map[string]string {
	out := map[string]string{}
	p.state(func(s *ownerState) {
		for _, d := range s.deferred {
			out[d.planned.ID.String()] = d.reason
		}
	})

	return out
}

// Phases of a slot's transaction as the tests name them; "" means none.
const (
	phaseBuilding = "building"
	phaseSent     = "sent"
)

func phaseOf(tx *liveTx) string {
	switch {
	case tx == nil:
		return ""
	case tx.phase == sent:
		return phaseSent
	default:
		return phaseBuilding
	}
}

// entry reads pl's slot entry: its transaction's phase and whether the slot is closed.
func (p *proc) entry(pl policy.Planned) (phase string, closed bool) {
	k := slotKey{window: WindowOf(pl.PlanTime), slot: slotOf(pl)}
	p.state(func(s *ownerState) {
		if e := s.entries[k]; e != nil {
			phase, closed = phaseOf(e.tx), e.closed
		}
	})

	return phase, closed
}

func TestQueueNotBlockedBySigner(t *testing.T) {
	h := newHarness(t, setup{})
	s := &gatedSigner{Signer: h.signer, gate: make(chan struct{})}
	release := time.AfterFunc(5*time.Second, func() { close(s.gate) })
	t.Cleanup(func() {
		if release.Stop() {
			close(s.gate)
		}
	})

	p := h.start(h.options(), s, nil)
	st := h.snapshot()

	for i := range 100 {
		began := time.Now()
		p.b.Queue(h.plan(st, i%(headRefund+1)))
		require.Less(t, time.Since(began), 100*time.Millisecond, "Queue call %d", i)
	}

	require.Eventually(t, func() bool { return s.signs.Load() >= 4 }, 5*time.Second, time.Millisecond,
		"the vetoes and the refund wait in the signer")
	require.Empty(t, h.node.Broadcasts())
}

func TestPriority(t *testing.T) {
	h := newHarness(t, setup{})
	s := &gatedSigner{Signer: h.signer, gate: make(chan struct{})}
	o := h.options()
	o.MaxInFlight = 1
	p := h.start(o, s, nil)
	st := h.snapshot()

	// The first veto takes the one in-flight place and waits in the signer.
	require.True(t, p.b.Queue(h.plan(st, firstVeto)))
	require.Eventually(t, func() bool { return s.signs.Load() == 1 }, 5*time.Second, time.Millisecond)

	vote := h.plan(st, 0)
	require.True(t, p.b.Queue(vote))
	p.b.Flush()
	require.True(t, p.b.Queue(h.plan(st, headRefund)))
	require.True(t, p.b.Queue(h.plan(st, firstVeto+1)))
	close(s.gate)

	// Each resolution frees the place for the next transaction.
	var order []string
	for n := 1; n <= 4; n++ {
		got := h.waitBroadcasts(n)
		order = append(order, got[n-1].ids...)
		h.commit()
	}

	require.Equal(t, []string{
		h.ids[firstVeto].String(), h.ids[firstVeto+1].String(), h.ids[headRefund].String(), h.ids[0].String(),
	}, order)
}

// vetoInSlot returns a veto, not valid on chain, of another transfer that shares p's single-message slot.
func vetoInSlot(t *testing.T, p policy.Planned) policy.Planned {
	t.Helper()

	for k := uint64(1_000); k < 1_000_000; k++ {
		id := p.ID
		id.Sequence = k
		q := policy.Planned{ID: id, Msg: &sinkv1.MsgVetoVoid{Signer: simnode.Reporter, ID: id.Proto(), Reason: "No Solana status"}}
		q.Key, q.PlanHeight, q.PlanTime = policy.KeyOf(q.ID, q.Msg), p.PlanHeight, p.PlanTime
		if slotOf(q) == slotOf(p) {
			return q
		}
	}
	t.Fatal("no veto shares the slot")

	return policy.Planned{}
}

func TestReleasedSlotTakesNewKind(t *testing.T) {
	h := newHarness(t, setup{})
	o := h.options()
	o.MaxInFlight = 1
	p := h.start(o, h.signer, nil)
	rev := &queuedHeads{release: make(chan error, 10)}
	for range 10 {
		rev.release <- nil
	}
	p.b.SetReviewer(rev)
	st := h.snapshot()

	// A veto of a transfer the chain does not have fails before submission and releases its slot.
	refund := h.plan(st, headRefund)
	stale := vetoInSlot(t, refund)
	require.True(t, p.b.Queue(stale))
	require.Eventually(t, func() bool { return p.count() == 1 }, 5*time.Second, time.Millisecond)
	_, errs := outcomes(t, p.lines())
	require.Contains(t, errs[stale.ID.String()], "message index: 0")
	phase, closed := p.entry(refund)
	require.Empty(t, phase)
	require.False(t, closed)

	// A vote holds the one in-flight place while the refund, in the released slot, and a veto queue.
	require.True(t, p.b.Queue(h.plan(st, 0)))
	p.b.Flush()
	h.waitBroadcasts(1)
	veto := h.plan(st, firstVeto)
	require.NotEqual(t, slotOf(refund), slotOf(veto))
	require.True(t, p.b.Queue(refund))
	require.True(t, p.b.Queue(veto))

	var order []string
	for n := 2; n <= 3; n++ {
		h.commit()
		got := h.waitBroadcasts(n)
		order = append(order, got[n-1].ids...)
	}
	require.Equal(t, []string{veto.ID.String(), refund.ID.String()}, order, "the veto goes before the refund")

	// The refund's inclusion reviews the refund head.
	h.commit()
	h.eventually(func() bool { return h.executions()[executedRefund(refund.ID)] == 1 }, "the refund is included")
	h.eventually(func() bool { return len(rev.calls()) == 1 }, "the refund head is reviewed")
}

func TestSlotBatching(t *testing.T) {
	t.Run("one transaction per slot", func(t *testing.T) {
		h := newHarness(t, setup{})
		o := h.options()
		o.MaxInFlight = 100
		p := h.start(o, h.signer, nil)
		st := h.snapshot()
		w := WindowOf(st.Time)

		want := map[int][]string{}
		votes := make([]policy.Planned, mixVotes)
		for i := range votes {
			votes[i] = h.plan(st, i)
			require.True(t, p.b.Queue(votes[i]))
			want[slotOf(votes[i])] = append(want[slotOf(votes[i])], votes[i].ID.String())
		}
		veto, refund := h.plan(st, firstVeto), h.plan(st, headRefund)
		require.True(t, p.b.Queue(veto))
		require.True(t, p.b.Queue(refund))
		p.b.Flush()

		got := h.waitBroadcasts(len(want) + 2)
		seen := map[int]bool{}
		singles := 0
		for _, d := range got {
			slot := -1
			for _, msg := range d.msgs {
				id, err := sinkid.Parse(msgID(t, msg))
				require.NoError(t, err)
				s := SlotOf(id, policy.KeyOf(id, msg))
				if slot >= 0 {
					require.Equal(t, slot, s, "one slot per transaction")
				}
				slot = s
			}
			require.True(t, SlotTimeout(w, slot).Equal(d.timeout), "the slot's timeout")

			if _, ok := d.msgs[0].(*sinkv1.MsgReportOutcome); !ok {
				singles++
				require.Len(t, d.msgs, 1)
				require.GreaterOrEqual(t, slot, VoteSlots)
				require.Less(t, slot, VoteSlots+SingleSlots)
				continue
			}

			require.False(t, seen[slot], "slot %d in two transactions", slot)
			seen[slot] = true
			require.ElementsMatch(t, want[slot], d.ids)
		}
		require.Equal(t, 2, singles)
		require.Len(t, seen, len(want))

		// A key already sent in its slot is skipped and changes nothing.
		require.False(t, p.b.Queue(votes[0]))
		require.False(t, p.b.Queue(veto))
		require.Empty(t, p.deferred())
		p.b.Flush()
		h.tick()
		require.Len(t, h.node.Broadcasts(), len(got))
	})

	t.Run("a full slot defers", func(t *testing.T) {
		h := newHarness(t, setup{})
		s := &gatedSigner{Signer: h.signer, gate: make(chan struct{})}
		t.Cleanup(func() { close(s.gate) })
		o := h.options()
		o.MaxInFlight = 1
		p := h.start(o, s, nil)
		st := h.snapshot()

		// The veto holds the in-flight place, so the full vote batch stays queued.
		require.True(t, p.b.Queue(h.plan(st, firstVeto)))
		require.Eventually(t, func() bool { return s.signs.Load() == 1 }, 5*time.Second, time.Millisecond)

		first := h.plan(st, 0)
		batch := append([]policy.Planned{first}, sameSlot(t, first, 25)...)
		for _, v := range batch[:25] {
			require.True(t, p.b.Queue(v))
		}
		require.False(t, p.b.Queue(batch[0]), "a key already queued is skipped")
		require.Empty(t, p.deferred())

		require.False(t, p.b.Queue(batch[25]))
		require.Equal(t, map[string]string{first.ID.String(): reasonSlotFull}, p.deferred())
	})
}

func TestSendTriggers(t *testing.T) {
	t.Run("flush", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)

		require.True(t, p.b.Queue(h.plan(h.snapshot(), 0)))
		for range 3 {
			h.tick()
		}
		require.Empty(t, h.node.Broadcasts(), "a partial batch waits")

		p.b.Flush()
		require.Len(t, h.waitBroadcasts(1)[0].msgs, 1)
	})

	t.Run("full", func(t *testing.T) {
		h := newHarness(t, setup{})
		o := h.options()
		o.MaxMsgs = 2
		p := h.start(o, h.signer, nil)

		pair := h.sameSlotVotes(h.snapshot(), 2)
		require.True(t, p.b.Queue(pair[0]))
		h.tick()
		require.Empty(t, h.node.Broadcasts())
		require.True(t, p.b.Queue(pair[1]))
		require.Len(t, h.waitBroadcasts(1)[0].msgs, 2)
	})

	t.Run("flush lead", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)

		vote := h.plan(h.snapshot(), 0)
		require.True(t, p.b.Queue(vote))
		timeout := SlotTimeout(WindowOf(vote.PlanTime), slotOf(vote))

		h.blockAt(timeout.Add(-31 * time.Second))
		for range 2 {
			h.tick()
		}
		require.Empty(t, h.node.Broadcasts(), "31 s before the timeout is before the flush lead")

		h.blockAt(timeout.Add(-28500 * time.Millisecond))
		h.eventually(func() bool { return len(h.node.Broadcasts()) == 1 }, "sent at its flush lead")
	})
}

func TestCutoff(t *testing.T) {
	for _, c := range []struct {
		name          string
		signerTimeout time.Duration
		left          time.Duration
		sent          bool
	}{
		{"keyring 19s", 0, 19 * time.Second, false},
		{"keyring 21s", 0, 21 * time.Second, true},
		{"remote 29s", 10 * time.Second, 29 * time.Second, false},
		{"remote 31s", 10 * time.Second, 31 * time.Second, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			o := h.options()
			o.SignerTimeout = c.signerTimeout
			p := h.start(o, h.signer, nil)

			early := h.plan(h.snapshot(), headRefund)
			timeout := SlotTimeout(WindowOf(early.PlanTime), slotOf(early))
			h.blockAt(timeout.Add(-c.left))
			h.clock.Advance(time.Millisecond)

			st := h.snapshot()
			require.True(t, st.Time.Equal(timeout.Add(-c.left)))
			refund := h.plan(st, headRefund)

			require.Equal(t, c.sent, p.b.Queue(refund))
			if c.sent {
				h.waitBroadcasts(1)
				return
			}
			require.Equal(t, map[string]string{refund.ID.String(): reasonCutoff}, p.deferred())
			h.tick()
			require.Empty(t, h.node.Broadcasts())
		})
	}
}

func TestCutoffRecheckedBeforeSigning(t *testing.T) {
	for _, c := range []struct {
		name string
		sim  time.Duration
		sent bool
	}{
		{"cutoff passes during simulation", 12 * time.Second, false},
		{"cutoff still holds after simulation", time.Second, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := &recordingSigner{Signer: reporterSigner(t)}
			timeout := start.Add(time.Minute)
			st := &akash.State{Height: 42, Time: timeout.Add(-21 * time.Second), Policy: sinkv1.ReporterPolicy{Members: []string{s.Address().String()}}}
			msg := voteMsg(s).(*sinkv1.MsgReportOutcome)
			id, err := sinkid.FromProto(msg.ID)
			require.NoError(t, err)
			vote := plannedAt(st, id, msg)

			chain := &capBuildChain{state: st}
			chain.sim = func([]byte) (uint64, error) {
				later := *chain.state
				later.Time = later.Time.Add(c.sim) // the chain moves on while simulation runs
				chain.state = &later
				return 100_000, nil
			}
			opts := Options{Reporter: s.Address().String(), GasAdjustment: 1.3, GasPrices: testPrices, MaxGas: 20_000_000}
			b := New(opts, chain, NewBuilder(testTxConfig(), testChainID, testAccountNumber, s), nil)
			b.stop, b.work = t.Context(), t.Context()
			logger := zerolog.Nop()
			b.log = &logger

			res := b.build(buildJob{msgs: []policy.Planned{vote}, timeout: timeout, chainTime: st.Time})
			if c.sent {
				require.NotNil(t, res.sent, "20 s left: still signed")
				return
			}
			require.Nil(t, res.sent, "not signed past the cutoff")
			require.Empty(t, s.got)
			require.Empty(t, chain.broadcasts)
			require.Len(t, res.deferred, 1)
			require.Equal(t, reasonCutoff, res.deferred[0].reason)
			require.True(t, res.seen.Time.Equal(timeout.Add(-9*time.Second)), "the owner sees the later block")
		})
	}
}

func TestStopBeforeSigningReleases(t *testing.T) {
	s := &recordingSigner{Signer: reporterSigner(t)}
	timeout := start.Add(time.Minute)
	st := &akash.State{Height: 42, Time: timeout.Add(-21 * time.Second), Policy: sinkv1.ReporterPolicy{Members: []string{s.Address().String()}}}
	msg := voteMsg(s).(*sinkv1.MsgReportOutcome)
	id, err := sinkid.FromProto(msg.ID)
	require.NoError(t, err)
	vote := plannedAt(st, id, msg)

	stop, shutdown := context.WithCancel(t.Context())
	chain := &capBuildChain{state: st}
	chain.sim = func([]byte) (uint64, error) {
		later := *chain.state
		later.Time = later.Time.Add(12 * time.Second) // the cutoff passes while simulation runs
		chain.state = &later
		shutdown() // and so does the stop
		return 100_000, nil
	}
	opts := Options{Reporter: s.Address().String(), GasAdjustment: 1.3, GasPrices: testPrices, MaxGas: 20_000_000}
	b := New(opts, chain, NewBuilder(testTxConfig(), testChainID, testAccountNumber, s), nil)
	b.stop, b.work = stop, t.Context()
	logger := zerolog.Nop()
	b.log = &logger

	res := b.build(buildJob{msgs: []policy.Planned{vote}, timeout: timeout, chainTime: st.Time})
	require.Empty(t, res.deferred, "a deferral would outlive shutdown")
	require.Equal(t, []policy.Planned{vote}, res.released)
	require.Nil(t, res.sent)
	require.Empty(t, s.got)
	require.Empty(t, chain.broadcasts)
}

func TestEarlierWindowNeverSent(t *testing.T) {
	h := newHarness(t, setup{})
	s := &gatedSigner{Signer: h.signer}
	p := h.start(h.options(), s, nil)

	st := h.snapshot()
	w := WindowOf(st.Time)
	queued, early := h.plan(st, 0), h.plan(st, 1)
	require.True(t, p.b.Queue(queued), "a partial batch of window w")

	// A block of the next window is seen.
	h.blockAt(WindowStart(w + 1).Add(time.Second))
	h.eventually(func() bool { return len(p.deferred()) == 1 }, "the queued vote is deferred once the window has passed")

	require.False(t, p.b.Queue(early), "a plan from window w after a block of w+1")
	require.Equal(t, map[string]string{queued.ID.String(): reasonCutoff, early.ID.String(): reasonCutoff}, p.deferred())

	p.b.Flush()
	h.tick()
	require.Zero(t, s.signs.Load(), "never signed")
	require.Empty(t, h.node.Broadcasts())
	require.Empty(t, p.lines(), "a deferral writes no event")
}

func TestSlotTaken(t *testing.T) {
	t.Run("refused on the first send", func(t *testing.T) {
		h := newHarness(t, setup{change: func(sc *simnode.Scenario) {
			sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultNonceTaken}}
		}})
		p := h.start(h.options(), h.signer, nil)
		veto := h.plan(h.snapshot(), firstVeto)

		require.True(t, p.b.Queue(veto))
		h.waitBroadcasts(1)
		require.Eventually(t, func() bool { return len(p.deferred()) == 1 }, 5*time.Second, time.Millisecond)
		require.Equal(t, map[string]string{veto.ID.String(): reasonSlotBusy}, p.deferred())
		phase, closed := p.entry(veto)
		require.Empty(t, phase)
		require.True(t, closed, "the slot is closed for the window")

		other := sameSlot(t, veto, 1)[0]
		require.False(t, p.b.Queue(other))
		require.Equal(t, reasonSlotBusy, p.deferred()[other.ID.String()])
		require.False(t, p.b.Queue(veto))

		h.tick()
		require.Len(t, h.node.Broadcasts(), 1)
		require.Empty(t, p.lines(), "no event")
	})

	t.Run("a rival executes first", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		vote := h.plan(h.snapshot(), 0)

		// Another process's transaction with the same slot timeout and different bytes reached the node first.
		timeout := SlotTimeout(WindowOf(vote.PlanTime), slotOf(vote))
		rival, _, err := h.builder(h.signer).Build(t.Context(), []sdk.Msg{vote.Msg}, 123_456, sdk.NewCoins(sdk.NewInt64Coin("uakt", 9)), timeout)
		require.NoError(t, err)
		h.node.Gossip(rival)

		require.True(t, p.b.Queue(vote))
		p.b.Flush()
		h.waitBroadcasts(1)

		h.node.CommitBlock()
		h.eventually(func() bool { return p.deferred()[vote.ID.String()] == reasonSlotBusy }, "deferred after the replay-key failure")

		require.Empty(t, p.lines(), "deferred, not failed")
		require.Equal(t, 1, h.executions()[executedVote(vote.ID)])
		_, closed := p.entry(vote)
		require.True(t, closed)
	})
}

// reviewCall is one follow-up review.
type reviewCall struct {
	ids       []string
	minHeight int64
}

type recordingReviewer struct {
	Reviewer
	mu    sync.Mutex
	calls []reviewCall
}

func (r *recordingReviewer) Review(ctx context.Context, ids []sinkid.ID, minHeight int64) error {
	call := reviewCall{minHeight: minHeight}
	for _, id := range ids {
		call.ids = append(call.ids, id.String())
	}

	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()

	return r.Reviewer.Review(ctx, ids, minHeight)
}

func (r *recordingReviewer) all() []reviewCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]reviewCall(nil), r.calls...)
}

func TestDeferredReviewedNextWindow(t *testing.T) {
	h := newHarness(t, setup{})
	p := h.start(h.options(), h.signer, nil)
	rec := &recordingReviewer{}
	p.wire(func(s *scan.Scanner) Reviewer { rec.Reviewer = s; return rec })

	vote := h.plan(h.snapshot(), 0)
	w := WindowOf(vote.PlanTime)
	timeout := SlotTimeout(w, slotOf(vote))
	rival, _, err := h.builder(h.signer).Build(t.Context(), []sdk.Msg{vote.Msg}, 123_456, sdk.NewCoins(sdk.NewInt64Coin("uakt", 9)), timeout)
	require.NoError(t, err)
	h.node.Gossip(rival)

	require.True(t, p.b.Queue(vote))
	p.b.Flush()
	h.waitBroadcasts(1)
	h.node.CommitBlock()
	h.eventually(func() bool { return len(p.deferred()) == 1 }, "deferred")
	require.Empty(t, rec.all(), "no review in the same window")

	h.blockAt(WindowStart(w + 1).Add(time.Second))
	first := h.node.Executions()
	h.eventually(func() bool { return len(rec.all()) == 1 && len(p.deferred()) == 0 }, "reviewed in the next window")

	st := h.snapshot()
	require.Equal(t, []reviewCall{{ids: []string{vote.ID.String()}, minHeight: st.Height}}, rec.all())

	// The vote executed through the rival, so the review plans nothing.
	lines := p.lines()
	require.Len(t, lines, 1)
	var review events.Review
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &review))
	require.Equal(t, vote.ID.String(), review.ID)
	require.Empty(t, review.Messages)

	for range 3 {
		h.tick()
	}
	require.Len(t, h.node.Broadcasts(), 1)
	require.Equal(t, first, h.node.Executions())
	require.Equal(t, 1, h.executions()[executedVote(vote.ID)])
	require.Empty(t, p.lines())
}

// gatedReviewer holds each review until gate is closed.
type gatedReviewer struct {
	Reviewer
	gate  chan struct{}
	calls atomic.Int64
	ended atomic.Int64
}

func (r *gatedReviewer) Review(ctx context.Context, ids []sinkid.ID, minHeight int64) error {
	r.calls.Add(1)
	defer r.ended.Add(1)
	<-r.gate

	return r.Reviewer.Review(ctx, ids, minHeight)
}

func TestFollowUpSendsOnlyItsBatches(t *testing.T) {
	t.Run("a pass and a follow-up fill different slots", func(t *testing.T) {
		h := newHarness(t, setup{change: func(sc *simnode.Scenario) {
			sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultNonceTaken}}
		}})
		p := h.start(h.options(), h.signer, nil)
		rev := &gatedReviewer{gate: make(chan struct{})}
		p.wire(func(s *scan.Scanner) Reviewer { rev.Reviewer = s; return rev })

		// X is deferred in window w: its slot is taken.
		st := h.snapshot()
		w := WindowOf(st.Time)
		pair := h.sameSlotVotes(st, 2)
		var x policy.Planned
		for i := range mixVotes {
			if c := h.plan(st, i); slotOf(c) != slotOf(pair[0]) && !containsID(pair, c.ID) {
				x = c
				break
			}
		}
		require.True(t, p.b.Queue(x))
		p.b.Flush()
		require.Eventually(t, func() bool { return p.deferred()[x.ID.String()] == reasonSlotBusy }, 5*time.Second, time.Millisecond)

		// In window w+1 a pass starts filling slot A while the follow-up for X waits to start.
		h.blockAt(WindowStart(w + 1).Add(time.Second))
		h.tick()
		next := h.snapshot()
		require.Equal(t, w+1, WindowOf(next.Time))
		pair = h.sameSlotVotes(next, 2)
		require.True(t, p.b.Queue(pair[0]))
		h.eventually(func() bool { return rev.calls.Load() == 1 }, "the follow-up starts")

		// The follow-up queues X into its own slot and ends: only X goes out.
		close(rev.gate)
		require.Eventually(t, func() bool { return rev.ended.Load() == 1 }, 5*time.Second, time.Millisecond)
		got := h.waitBroadcasts(2)
		require.Equal(t, []string{x.ID.String()}, got[1].ids)
		for range 3 {
			h.tick()
		}
		require.Len(t, h.node.Broadcasts(), 2, "the pass's batch keeps filling")

		// The pass's later vote for its slot joins the batch, and the batch goes out when the pass ends.
		require.True(t, p.b.Queue(pair[1]))
		require.Empty(t, p.deferred())
		p.b.Flush()
		require.ElementsMatch(t, []string{pair[0].ID.String(), pair[1].ID.String()}, h.waitBroadcasts(3)[2].ids)
	})

	t.Run("a batch both filled waits for the later review", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		pair := h.sameSlotVotes(h.snapshot(), 2)
		follow := followQueuer{b: p.b, job: 7}

		require.True(t, p.b.Queue(pair[0]))
		require.True(t, follow.Queue(pair[1]))

		p.b.Flush()
		for range 3 {
			h.tick()
		}
		require.Empty(t, h.node.Broadcasts(), "the follow-up still fills the batch")

		require.True(t, p.b.call(func() { p.b.reviewEnded(follow.job) }))
		require.Len(t, h.waitBroadcasts(1)[0].ids, 2)
	})

	t.Run("the flush lead does not wait for reviews", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		vote := h.plan(h.snapshot(), 0)

		require.True(t, followQueuer{b: p.b, job: 7}.Queue(vote))
		h.blockAt(SlotTimeout(WindowOf(vote.PlanTime), slotOf(vote)).Add(-29 * time.Second))
		h.eventually(func() bool { return len(h.node.Broadcasts()) == 1 }, "sent at its flush lead")
	})
}

func TestInFlightStates(t *testing.T) {
	// requeue checks that a released message can be queued again and is then sent.
	requeue := func(t *testing.T, h *harness, p *proc, pl policy.Planned, fix func()) {
		t.Helper()

		phase, _ := p.entry(pl)
		require.Empty(t, phase, "released")
		require.Empty(t, p.deferred())

		fix()
		h.tick()
		require.True(t, p.b.Queue(pl), "a released message can be queued again")
		p.b.Flush()
		require.Eventually(t, func() bool { phase, _ := p.entry(pl); return phase == phaseSent }, 5*time.Second, time.Millisecond)
	}

	t.Run("simulation", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		vote := h.plan(h.snapshot(), 0)
		h.node.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.SimFailures = []simnode.SimFailure{{ID: vote.ID.String(), Error: "transfer is busy"}}
		})

		require.True(t, p.b.Queue(vote))
		p.b.Flush()
		require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
		_, errs := outcomes(t, p.lines())
		require.Contains(t, errs[vote.ID.String()], "message index: 0: transfer is busy")
		require.Empty(t, h.node.Broadcasts())

		requeue(t, h, p, vote, func() { h.node.Mutate(func(sc *simnode.Scenario) { sc.Akash.SimFailures = nil }) })
	})

	t.Run("signing", func(t *testing.T) {
		h := newHarness(t, setup{})
		s := &gatedSigner{Signer: h.signer}
		text := "signer unavailable"
		s.fail.Store(&text)
		p := h.start(h.options(), s, nil)
		veto := h.plan(h.snapshot(), firstVeto)

		require.True(t, p.b.Queue(veto))
		require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
		_, errs := outcomes(t, p.lines())
		require.Equal(t, map[string]string{veto.ID.String(): text}, errs)

		requeue(t, h, p, veto, func() { s.fail.Store(nil) })
	})

	t.Run("membership", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		veto := h.plan(h.snapshot(), firstVeto)
		var members []string
		h.node.Mutate(func(sc *simnode.Scenario) {
			members = sc.Akash.Policies[0].Members
			sc.Akash.Policies[0].Members = members[1:]
		})
		h.tick()

		require.True(t, p.b.Queue(veto))
		require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
		_, errs := outcomes(t, p.lines())
		require.Equal(t, map[string]string{veto.ID.String(): "reporter was removed"}, errs)

		requeue(t, h, p, veto, func() { h.node.Mutate(func(sc *simnode.Scenario) { sc.Akash.Policies[0].Members = members }) })
	})

	t.Run("signer identity", func(t *testing.T) {
		h := newHarness(t, setup{})
		o := h.options()
		o.Reporter = h.ids[0].Owner
		p := h.start(o, h.signer, nil)
		h.node.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.Policies[0].Members = append(sc.Akash.Policies[0].Members, o.Reporter)
		})
		h.tick()
		veto := h.plan(h.snapshot(), firstVeto)

		require.True(t, p.b.Queue(veto))
		require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
		_, errs := outcomes(t, p.lines())
		require.Equal(t, map[string]string{veto.ID.String(): "signer " + simnode.Reporter + " is not reporter " + o.Reporter}, errs)
		phase, _ := p.entry(veto)
		require.Empty(t, phase)
	})

	t.Run("cap", func(t *testing.T) {
		h := newHarness(t, setup{})
		o := h.options()
		o.MaxFee = sdk.NewInt64Coin("uakt", 1_000)
		p := h.start(o, h.signer, nil)
		veto := h.plan(h.snapshot(), firstVeto)

		require.True(t, p.b.Queue(veto))
		require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
		_, errs := outcomes(t, p.lines())
		require.Equal(t, map[string]string{veto.ID.String(): "fee 3250uakt exceeds tx.max-fee 1000uakt"}, errs)
		phase, _ := p.entry(veto)
		require.Empty(t, phase)
		require.True(t, p.b.Queue(veto), "a released message can be queued again")
	})

	t.Run("a sent entry is kept until inclusion", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		veto := h.plan(h.snapshot(), firstVeto)

		require.True(t, p.b.Queue(veto))
		h.waitBroadcasts(1)
		for range 3 {
			h.tick()
		}
		phase, _ := p.entry(veto)
		require.Equal(t, phaseSent, phase)

		h.commit()
		h.eventually(func() bool { phase, _ := p.entry(veto); return phase == "" }, "resolved")
		_, closed := p.entry(veto)
		require.True(t, closed)
		hashes, _ := outcomes(t, p.lines())
		require.Contains(t, hashes, veto.ID.String())
	})
}

func TestResendsAfterLostResponse(t *testing.T) {
	for _, c := range []struct {
		name   string
		faults []simnode.BroadcastFault
	}{
		{"already in cache", []simnode.BroadcastFault{{Kind: simnode.FaultLostResponse}}},
		{"nonce taken by the earlier copy", []simnode.BroadcastFault{{Kind: simnode.FaultLostResponse}, {Kind: simnode.FaultNonceTaken}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, setup{change: func(sc *simnode.Scenario) { sc.Akash.BroadcastFaults = c.faults }})
			p := h.start(h.options(), h.signer, nil)
			veto := h.plan(h.snapshot(), firstVeto)

			require.True(t, p.b.Queue(veto))
			first := h.waitBroadcasts(1)[0]
			h.eventually(func() bool { return len(h.node.Broadcasts()) == 2 }, "re-sent")
			require.Equal(t, first.raw, h.broadcasts()[1].raw, "the same bytes")

			for range 3 {
				h.tick()
			}
			require.Len(t, h.node.Broadcasts(), 2, "no re-send loop")
			phase, _ := p.entry(veto)
			require.Equal(t, phaseSent, phase)

			h.commit()
			h.eventually(func() bool { phase, _ := p.entry(veto); return phase == "" }, "confirmed")
			hashes, errs := outcomes(t, p.lines())
			require.Equal(t, map[string]string{veto.ID.String(): first.hash()}, hashes)
			require.Empty(t, errs)
		})
	}
}

func TestFirstSendInCache(t *testing.T) {
	h := newHarness(t, setup{})
	a := h.start(h.options(), h.signer, nil)
	b := h.start(h.options(), h.signer, h.dial())
	refund := h.plan(h.snapshot(), headRefund)

	require.True(t, a.b.Queue(refund))
	h.waitBroadcasts(1)
	require.True(t, b.b.Queue(refund))
	got := h.waitBroadcasts(2)
	require.Equal(t, got[0].raw, got[1].raw, "byte-identical")
	phase, _ := b.entry(refund)
	require.Equal(t, phaseSent, phase)

	for range 3 {
		h.tick()
	}
	require.Len(t, h.node.Broadcasts(), 2, "no re-send loop")

	h.commit()
	for _, p := range []*proc{a, b} {
		h.eventually(func() bool { phase, _ := p.entry(refund); return phase == "" }, "confirmed")
		hashes, errs := outcomes(t, p.lines())
		require.Equal(t, map[string]string{refund.ID.String(): got[0].hash()}, hashes)
		require.Empty(t, errs)
	}
	require.Len(t, h.executions(), 1)
}

func TestFirstSendRejectionIsFinal(t *testing.T) {
	const log = "insufficient funds: 0uakt is smaller than 3250uakt"
	h := newHarness(t, setup{change: func(sc *simnode.Scenario) {
		sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultReject, Code: 5, Codespace: replayCodespace, Log: log}}
	}})
	p := h.start(h.options(), h.signer, nil)
	pair := h.sameSlotVotes(h.snapshot(), 2)

	for _, v := range pair {
		require.True(t, p.b.Queue(v))
	}
	p.b.Flush()
	rejected := h.waitBroadcasts(1)[0]
	require.Eventually(t, func() bool { return p.count() == 2 }, 5*time.Second, time.Millisecond)

	_, errs := outcomes(t, p.lines())
	text := "sink transaction rejected: " + rejected.hash() + " code 5: " + log
	require.Equal(t, map[string]string{pair[0].ID.String(): text, pair[1].ID.String(): text}, errs)
	phase, closed := p.entry(pair[0])
	require.Empty(t, phase)
	require.False(t, closed, "the slot stays usable in the window")

	// A new plan for the same slot in the same window is sent.
	require.True(t, p.b.Queue(pair[0]))
	p.b.Flush()
	require.Equal(t, []string{pair[0].ID.String()}, h.waitBroadcasts(2)[1].ids)
	h.commit()
	h.eventually(func() bool { phase, _ := p.entry(pair[0]); return phase == "" }, "confirmed")
	hashes, _ := outcomes(t, p.lines())
	require.Contains(t, hashes, pair[0].ID.String())
}

func TestResendRejectionNotFinal(t *testing.T) {
	h := newHarness(t, setup{change: func(sc *simnode.Scenario) {
		sc.Akash.BroadcastFaults = []simnode.BroadcastFault{
			{Kind: simnode.FaultLostResponse},
			{Kind: simnode.FaultReject, Code: 5, Codespace: replayCodespace, Log: "insufficient funds"},
		}
	}})
	p := h.start(h.options(), h.signer, nil)
	veto := h.plan(h.snapshot(), firstVeto)

	require.True(t, p.b.Queue(veto))
	first := h.waitBroadcasts(1)[0]
	h.eventually(func() bool { return len(h.node.Broadcasts()) == 2 }, "re-sent")
	for range 3 {
		h.tick()
	}
	phase, _ := p.entry(veto)
	require.Equal(t, phaseSent, phase, "a rejected re-send stays sent")
	require.Len(t, h.node.Broadcasts(), 2)
	require.Empty(t, p.lines())

	h.commit()
	h.eventually(func() bool { phase, _ := p.entry(veto); return phase == "" }, "resolved")
	hashes, errs := outcomes(t, p.lines())
	require.Equal(t, map[string]string{veto.ID.String(): first.hash()}, hashes)
	require.Empty(t, errs)
}

func TestConfirmationEvents(t *testing.T) {
	t.Run("included", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		pair := h.sameSlotVotes(h.snapshot(), 2)

		for _, v := range pair {
			require.True(t, p.b.Queue(v))
		}
		p.b.Flush()
		d := h.waitBroadcasts(1)[0]
		h.commit()
		h.eventually(func() bool { return p.count() == 2 }, "confirmed")

		hashes, errs := outcomes(t, p.lines())
		require.Empty(t, errs)
		require.Equal(t, map[string]string{pair[0].ID.String(): d.hash(), pair[1].ID.String(): d.hash()}, hashes)
		require.Equal(t, strings.ToUpper(d.hash()), d.hash())
		require.Len(t, d.hash(), 64)
	})

	t.Run("failed delivery", func(t *testing.T) {
		h := newHarness(t, setup{change: func(sc *simnode.Scenario) {
			sc.Akash.Deliveries = []simnode.Delivery{{Code: 5, Codespace: replayCodespace, Log: "insufficient funds"}}
		}})
		p := h.start(h.options(), h.signer, nil)
		veto := h.plan(h.snapshot(), firstVeto)

		require.True(t, p.b.Queue(veto))
		d := h.waitBroadcasts(1)[0]
		h.commit()
		h.eventually(func() bool { return p.count() == 1 }, "resolved")

		hashes, errs := outcomes(t, p.lines())
		require.Empty(t, hashes)
		require.Equal(t, map[string]string{veto.ID.String(): "sink transaction failed: " + d.hash() + " code 5"}, errs)
		require.Equal(t, 1, p.b.TakeFailures())
	})
}

// failingLookups fails every transaction lookup while down is set.
type failingLookups struct {
	Chain
	down  *atomic.Bool
	calls atomic.Int64
}

func (c *failingLookups) Tx(ctx context.Context, hash []byte) (*akash.TxResult, error) {
	c.calls.Add(1)
	if c.down.Load() {
		return nil, status.Error(codes.Unavailable, "connection refused")
	}

	return c.Chain.Tx(ctx, hash)
}

func TestFailedLookupRetriedAtSameHeight(t *testing.T) {
	h := newHarness(t, setup{})
	var down atomic.Bool
	down.Store(true)
	lookups := &failingLookups{down: &down}
	p := h.start(h.options(), h.signer, nil, func(c Chain) Chain { lookups.Chain = c; return lookups })
	veto := h.plan(h.snapshot(), firstVeto)

	require.True(t, p.b.Queue(veto))
	h.waitBroadcasts(1)
	h.commit() // includes the veto; no later block follows
	height := h.snapshot().Height
	h.eventually(func() bool { return lookups.calls.Load() >= 2 }, "looked up at the inclusion height")

	time.Sleep(50 * time.Millisecond)
	n := lookups.calls.Load()
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, n, lookups.calls.Load(), "no retry without a refresh tick")

	down.Store(false)
	h.eventually(func() bool { return p.count() == 1 }, "confirmed at the same height")
	require.Equal(t, height, h.snapshot().Height, "no new block")
	hashes, errs := outcomes(t, p.lines())
	require.Contains(t, hashes, veto.ID.String())
	require.Empty(t, errs)
}

func TestExpiryByChainTime(t *testing.T) {
	faults := make([]simnode.BroadcastFault, 500)
	for i := range faults {
		faults[i] = simnode.BroadcastFault{Kind: simnode.FaultTransport}
	}
	h := newHarness(t, setup{change: func(sc *simnode.Scenario) { sc.Akash.BroadcastFaults = faults }})
	o := h.options()
	o.RefreshInterval = 10 * time.Millisecond
	h.step = o.RefreshInterval
	p := h.start(o, h.signer, nil)
	veto := h.plan(h.snapshot(), firstVeto)
	w := WindowOf(veto.PlanTime)
	timeout := SlotTimeout(w, slotOf(veto))

	require.True(t, p.b.Queue(veto))
	d := h.waitBroadcasts(1)[0]
	h.eventually(func() bool { return len(h.node.Broadcasts()) >= 3 }, "re-sent after transport errors")

	h.blockAt(timeout)
	for range 3 {
		h.tick()
	}
	require.Empty(t, p.lines(), "a block at the timeout itself does not expire it")

	h.blockAt(timeout.Add(50 * time.Millisecond))
	require.Equal(t, w, WindowOf(timeout.Add(50*time.Millisecond)), "still in the window")
	h.eventually(func() bool { return p.count() == 1 }, "expired")
	_, errs := outcomes(t, p.lines())
	require.Equal(t, map[string]string{veto.ID.String(): "sink transaction expired: " + d.hash()}, errs)

	resent := len(h.node.Broadcasts())
	h.blockAt(WindowStart(w + 1).Add(time.Second))
	h.eventually(func() bool {
		left := false
		p.state(func(s *ownerState) {
			for k := range s.entries {
				left = left || k.window == w
			}
		})
		return !left
	}, "the window's entries leave the table")
	require.Len(t, h.node.Broadcasts(), resent, "no re-send after the timeout")
}

func TestHaltedChain(t *testing.T) {
	// prepare sends the veto and defers a vote whose slot is taken, then halts the chain.
	prepare := func(t *testing.T) (*harness, *proc, policy.Planned, policy.Planned) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		st := h.snapshot()
		veto, vote := h.plan(st, firstVeto), h.plan(st, 0)

		require.True(t, p.b.Queue(veto))
		h.waitBroadcasts(1)
		h.node.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultNonceTaken}}
		})
		require.True(t, p.b.Queue(vote))
		p.b.Flush()
		require.Eventually(t, func() bool { return len(p.deferred()) == 1 }, 5*time.Second, time.Millisecond)
		h.node.Halt()

		return h, p, veto, vote
	}

	t.Run("scan reports both at the waiting limit", func(t *testing.T) {
		h, p, veto, vote := prepare(t)
		sent := h.broadcasts()[0]

		done := make(chan int, 1)
		go func() { done <- p.b.Wait(t.Context()) }()

		var failures int
		finished := false
		for i := 0; i < 40 && !finished; i++ {
			h.clock.Advance(10 * time.Second)
			select {
			case failures = <-done:
				finished = true
			case <-time.After(20 * time.Millisecond):
			}
		}
		require.True(t, finished, "Wait returns at the waiting limit")
		require.Equal(t, 2, failures)

		_, errs := outcomes(t, p.lines())
		require.Equal(t, map[string]string{
			veto.ID.String(): "sink transaction unconfirmed: " + sent.hash(),
			vote.ID.String(): "sink message not sent: deferred (slot_busy) until " +
				SlotTimeout(WindowOf(vote.PlanTime), slotOf(vote)).Format(time.RFC3339Nano),
		}, errs)
		require.Contains(t, errs[vote.ID.String()], "until 2026-11-12T10:01:59.")
	})

	for _, halted := range []bool{true, false} {
		t.Run(fmt.Sprintf("a message still being built (halted %v)", halted), func(t *testing.T) {
			h := newHarness(t, setup{})
			var down atomic.Bool
			down.Store(true)
			o := h.options()
			o.WaitLimit = 20 * time.Second
			p := h.start(o, h.signer, nil, func(c Chain) Chain { return &unavailableSim{Chain: c, down: &down} })
			veto := h.plan(h.snapshot(), firstVeto)
			if halted {
				h.node.Halt()
			}

			// Chain time stands still, so the build retries the unavailable simulation without reaching its cutoff.
			require.True(t, p.b.Queue(veto))
			require.Eventually(t, func() bool { phase, _ := p.entry(veto); return phase == phaseBuilding }, 5*time.Second, time.Millisecond)

			done := make(chan int, 1)
			go func() { done <- p.b.Wait(t.Context()) }()
			var failures int
			finished := false
			for i := 0; i < 20 && !finished; i++ {
				h.clock.Advance(2 * time.Second)
				select {
				case failures = <-done:
					finished = true
				case <-time.After(20 * time.Millisecond):
				}
			}
			require.True(t, finished, "Wait returns at the waiting limit")
			require.Equal(t, 1, failures, "the building message counts")
			_, errs := outcomes(t, p.lines())
			require.Equal(t, map[string]string{
				veto.ID.String(): "sink message not sent: queued until " + SlotTimeout(WindowOf(veto.PlanTime), slotOf(veto)).Format(time.RFC3339Nano),
			}, errs)

			// The build sends after all: the transaction keeps its slot, and nothing more is written for the veto.
			down.Store(false)
			h.eventually(func() bool { phase, _ := p.entry(veto); return phase == phaseSent }, "sent after Wait gave up")
			require.Len(t, h.node.Broadcasts(), 1)
			require.False(t, p.b.Queue(veto), "the slot keeps the key")
			require.Zero(t, p.b.Wait(t.Context()), "an abandoned transaction is not awaited")

			if !halted {
				h.commit()
				h.eventually(func() bool { return h.executions()[executedVeto(veto.ID, 1)] == 1 }, "included")
			}
			for range 3 {
				h.tick()
			}
			require.Empty(t, p.lines(), "no second event for the veto")
			require.Zero(t, p.b.TakeFailures())
		})
	}

	t.Run("start keeps both", func(t *testing.T) {
		h, p, veto, vote := prepare(t)

		for range 30 {
			h.clock.Advance(10 * time.Second)
			time.Sleep(2 * time.Millisecond)
		}
		require.Equal(t, 0, p.b.TakeFailures())
		require.Empty(t, p.lines())
		phase, _ := p.entry(veto)
		require.Equal(t, phaseSent, phase)
		require.Equal(t, map[string]string{vote.ID.String(): reasonSlotBusy}, p.deferred())
	})
}

func TestAbortedScanReportsDeferred(t *testing.T) {
	h := newHarness(t, setup{change: func(sc *simnode.Scenario) {
		sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultNonceTaken}}
	}})
	p := h.start(h.options(), h.signer, nil)
	pin := simnode.LightClient
	s := h.pinnedScanner(p.chain, p.ev, p.b, p.b.o.RefreshInterval, &pin)
	p.b.SetReviewer(s)

	// A vote is deferred: its slot is taken.
	vote := h.plan(h.snapshot(), 0)
	require.True(t, p.b.Queue(vote))
	p.b.Flush()
	require.Eventually(t, func() bool { return p.deferred()[vote.ID.String()] == reasonSlotBusy }, 5*time.Second, time.Millisecond)

	// Governance re-pins the bridge, so the scan's pass ends on a pin mismatch while blocks keep coming.
	other := solana.PublicKeyFromBytes(bytes.Repeat([]byte{5}, 32))
	h.node.Mutate(func(sc *simnode.Scenario) {
		sc.Solana.Accounts[aktsolana.PDA(simnode.ProgramID, []byte("bridge"))] = simnode.Account{
			Owner: simnode.ProgramID, Lamports: 1, Data: simnode.EncodeBridgeConfig(simnode.BridgeConfig{
				LightClientProgram: other, ChainID: simnode.ChainID,
				PerTransferMax: 1_000_000_000_000, DailyMintCap: 1_000_000_000_000,
			}),
		}
	})

	done := make(chan int, 1)
	go func() { done <- s.Pass(t.Context(), p.settle(t.Context())) }()

	var failures int
	finished := false
	for i := 0; i < 100 && !finished; i++ {
		h.clock.Advance(10 * time.Second)
		h.node.CommitBlock()
		select {
		case failures = <-done:
			finished = true
		case <-time.After(20 * time.Millisecond):
		}
	}
	require.True(t, finished, "scan returns over 1,000 s of healthy blocks")
	require.Equal(t, 2, failures, "the deferred vote and the pass error")

	lines := p.lines()
	require.Len(t, lines, 3)
	_, errs := outcomes(t, lines[:1])
	require.Equal(t, map[string]string{
		vote.ID.String(): "sink message not sent: deferred (slot_busy) until " +
			SlotTimeout(WindowOf(vote.PlanTime), slotOf(vote)).Format(time.RFC3339Nano),
	}, errs)
	require.JSONEq(t, `{"alert":"light-client-mismatch","pinned":"`+pin.String()+`","current":"`+other.String()+`"}`, lines[1])
	require.Contains(t, lines[2], "differs from pinned")
	require.Empty(t, p.deferred())
}

func TestAbortedFollowUpReportsOnlyWhatItSkipped(t *testing.T) {
	h := newHarness(t, setup{})
	p := h.start(h.options(), h.signer, nil)
	st := h.snapshot()
	w := WindowOf(st.Time)
	a, b := h.plan(st, 0), h.plan(st, 1)
	until := func(pl policy.Planned) time.Time { return SlotTimeout(w, slotOf(pl)) }
	p.state(func(s *ownerState) {
		for _, pl := range []policy.Planned{a, b} {
			s.deferred[pl.Key] = &deferral{planned: pl, reason: reasonSlotBusy, window: w, until: until(pl)}
		}
	})

	// In window w+1 the follow-up queues a again, then a pass's pin mismatch stops it before b.
	h.blockAt(WindowStart(w + 1).Add(time.Second))
	h.tick()
	job := followJob{id: 9, window: w + 1, ids: []sinkid.ID{a.ID, b.ID}, keys: []policy.Key{a.Key, b.Key}}
	require.True(t, followQueuer{b: p.b, job: job.id}.Queue(h.plan(h.snapshot(), 0)))
	require.True(t, p.b.call(func() { p.b.followDone(job, scan.ErrPassAborted) }))

	_, errs := outcomes(t, p.lines())
	require.Equal(t, map[string]string{b.ID.String(): "sink message not sent: deferred (slot_busy) until " + rfc3339(until(b))}, errs)
	require.Equal(t, []string{a.ID.String()}, h.waitBroadcasts(1)[0].ids)
}

func TestWaitDoesNotSpin(t *testing.T) {
	h := newHarness(t, setup{})
	var pumps atomic.Int64
	o := h.options()
	o.changed = func(*ownerState) { pumps.Add(1) }
	p := h.start(o, h.signer, nil)

	// The veto stays unconfirmed: no block is committed and the fake clock stands still, so nothing ticks.
	require.True(t, p.b.Queue(h.plan(h.snapshot(), firstVeto)))
	h.waitBroadcasts(1)
	require.Eventually(t, func() bool { return p.entryOf(h.ids[firstVeto]) == phaseSent }, 5*time.Second, time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan int, 1)
	go func() { done <- p.b.Wait(ctx) }()

	time.Sleep(50 * time.Millisecond)
	before := pumps.Load()
	time.Sleep(200 * time.Millisecond)
	spun := pumps.Load() - before

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after its context ended")
	}
	require.Less(t, spun, int64(5), "Wait's own queries do not wake it")
}

// unavailableSim fails every simulation in transport while down is set.
type unavailableSim struct {
	Chain
	down *atomic.Bool
}

func (c *unavailableSim) Simulate(ctx context.Context, txBytes []byte) (uint64, error) {
	if c.down.Load() {
		return 0, status.Error(codes.Unavailable, "connection refused")
	}

	return c.Chain.Simulate(ctx, txBytes)
}

// logBuffer collects log lines; it is safe for concurrent writers.
type logBuffer struct{ output }

func TestNoTxIndex(t *testing.T) {
	h := newHarness(t, setup{change: func(sc *simnode.Scenario) { sc.Akash.NoTxIndex = true }})
	logs := &logBuffer{}
	ctx := zerolog.New(logs).WithContext(context.Background())
	p := h.startCtx(ctx, h.options(), h.signer, nil)
	s := p.wire(nil)

	failures := s.Pass(t.Context(), func() int {
		p.b.Flush()
		return p.b.Wait(t.Context())
	})
	require.Equal(t, 0, failures, "accepted transactions are no failures, and Wait does not wait for them")
	require.Len(t, h.node.Broadcasts(), len(h.voteSlots(h.snapshot()))+headRefund-firstVeto+1,
		"a transaction per vote slot, per veto and for the refund")

	lines := p.lines()
	hashes, errs := outcomes(t, lines)
	require.Empty(t, hashes, "no outcome event")
	require.Empty(t, errs)

	h.commit()
	st := h.snapshot()
	h.blockAt(WindowStart(WindowOf(st.Time) + 1).Add(time.Second))
	h.tick()
	require.Equal(t, 0, s.Pass(t.Context(), func() int { p.b.Flush(); return p.b.Wait(t.Context()) }))

	execs := h.executions()
	for i := range mixVotes {
		require.Equal(t, 1, execs[executedVote(h.ids[i])], "vote %d", i)
	}
	for i := firstVeto; i < headRefund; i++ {
		require.Equal(t, 1, execs[executedVeto(h.ids[i], 1)], "veto %d", i)
	}

	require.Equal(t, 1, strings.Count(logs.text(), `"level":"warn"`), logs.text())
	require.Contains(t, logs.text(), "does not index transactions")
}

// voteSlots counts broadcast-mix's votes from st per slot.
func (h *harness) voteSlots(st *akash.State) map[int]int {
	out := map[int]int{}
	for i := range mixVotes {
		out[slotOf(h.plan(st, i))]++
	}

	return out
}

func TestCapsSplitBatches(t *testing.T) {
	gasCases := []struct {
		name        string
		maxGas      uint64
		blockMaxGas int64
		over        string
	}{
		// With adjustment 1, simnode uses 60,000 + 40,000 per message: two messages 140,000, one 100,000.
		{"batch halved", 120_000, 0, ""},
		{"single over the cap", 90_000, 0, "gas limit 100000 exceeds tx.max-gas 90000"},
		{"batch halved at the block max gas", 20_000_000, 120_000, ""},
		{"single over the block max gas", 20_000_000, 90_000, "gas limit 100000 exceeds the block max gas 90000"},
		{"tx.max-gas under the block max gas", 90_000, 120_000, "gas limit 100000 exceeds tx.max-gas 90000"},
	}
	for _, c := range gasCases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, setup{change: func(sc *simnode.Scenario) { sc.Akash.BlockMaxGas = c.blockMaxGas }})
			o := h.options()
			o.GasAdjustment = 1
			o.MaxGas = c.maxGas
			o.BlockMaxGas = c.blockMaxGas
			p := h.start(o, h.signer, nil)
			pair := h.sameSlotVotes(h.snapshot(), 2)

			for _, v := range pair {
				require.True(t, p.b.Queue(v))
			}
			p.b.Flush()

			if c.over == "" {
				d := h.waitBroadcasts(1)[0]
				require.Equal(t, []string{pair[0].ID.String()}, d.ids)
				require.Equal(t, uint64(100_000), d.gas)
				require.Equal(t, map[string]string{pair[1].ID.String(): reasonCap}, p.deferred())
				return
			}

			require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
			_, errs := outcomes(t, p.lines())
			require.Equal(t, map[string]string{pair[0].ID.String(): c.over}, errs)
			require.Equal(t, map[string]string{pair[1].ID.String(): reasonCap}, p.deferred())
			require.Empty(t, h.node.Broadcasts())
		})
	}

	// signBytes measures the sign doc of one and two messages at the gas and fee they would carry.
	signBytes := func(h *harness, pair []policy.Planned) (one, two int) {
		b := h.builder(h.signer)
		timeout := SlotTimeout(WindowOf(pair[0].PlanTime), slotOf(pair[0]))
		size := func(ps []policy.Planned) int {
			gas := uint64(60_000+40_000*len(ps)) * 13 / 10
			n, err := b.SignBytesLen(sdkMsgs(ps), gas, FeeFor(gas, testPrices, nil), timeout)
			require.NoError(t, err)
			return n
		}

		return size(pair[:1]), size(pair)
	}

	t.Run("batches sized to the signer's limit", func(t *testing.T) {
		h := newHarness(t, setup{})
		pair := h.sameSlotVotes(h.snapshot(), 2)
		one, two := signBytes(h, pair)
		require.Less(t, one, two)
		p := h.start(h.options(), &gatedSigner{Signer: h.signer, limits: []int{one}}, nil)

		for _, v := range pair {
			require.True(t, p.b.Queue(v))
		}
		p.b.Flush()
		require.Equal(t, []string{pair[0].ID.String()}, h.waitBroadcasts(1)[0].ids)
		require.Equal(t, map[string]string{pair[1].ID.String(): reasonCap}, p.deferred())
	})

	t.Run("a batch the signer still refuses is halved", func(t *testing.T) {
		h := newHarness(t, setup{})
		pair := h.sameSlotVotes(h.snapshot(), 2)
		one, two := signBytes(h, pair)
		// The sizing check sees room for both; Build's own check then refuses the pair.
		s := &gatedSigner{Signer: h.signer, limits: []int{two, one, two, one}}
		p := h.start(h.options(), s, nil)

		for _, v := range pair {
			require.True(t, p.b.Queue(v))
		}
		p.b.Flush()
		require.Equal(t, []string{pair[0].ID.String()}, h.waitBroadcasts(1)[0].ids)
		require.Equal(t, map[string]string{pair[1].ID.String(): reasonCap}, p.deferred())
	})

	t.Run("a single message over the signer's limit", func(t *testing.T) {
		h := newHarness(t, setup{})
		pair := h.sameSlotVotes(h.snapshot(), 2)
		one, _ := signBytes(h, pair)
		p := h.start(h.options(), &gatedSigner{Signer: h.signer, limits: []int{one - 1}}, nil)

		require.True(t, p.b.Queue(pair[0]))
		p.b.Flush()
		require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
		_, errs := outcomes(t, p.lines())
		require.Equal(t, map[string]string{
			pair[0].ID.String(): fmt.Sprintf("sign bytes exceed the signer's limit: %d bytes, limit %d", one, one-1),
		}, errs)
		require.Empty(t, h.node.Broadcasts())
	})
}

func TestGasLimit(t *testing.T) {
	t.Run("auto", func(t *testing.T) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)

		require.True(t, p.b.Queue(h.plan(h.snapshot(), firstVeto)))
		d := h.waitBroadcasts(1)[0]
		require.Equal(t, uint64(130_000), d.gas, "ceil(100,000 × 1.3)")
		require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 3_250)), d.fee)
	})

	t.Run("per message", func(t *testing.T) {
		h := newHarness(t, setup{})
		o := h.options()
		o.GasPerMessage = 50_000
		p := h.start(o, h.signer, nil)

		for _, v := range h.sameSlotVotes(h.snapshot(), 3) {
			require.True(t, p.b.Queue(v))
		}
		p.b.Flush()
		d := h.waitBroadcasts(1)[0]
		require.Len(t, d.msgs, 3)
		require.Equal(t, uint64(150_000), d.gas)
	})
}

func TestUnorderedDisabled(t *testing.T) {
	t.Run("refused at CheckTx", func(t *testing.T) {
		const log = "unordered transactions are not enabled: feature not supported"
		h := newHarness(t, setup{change: func(sc *simnode.Scenario) {
			sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultReject, Code: 37, Codespace: replayCodespace, Log: log}}
		}})
		p := h.start(h.options(), h.signer, nil)
		veto := h.plan(h.snapshot(), firstVeto)

		require.True(t, p.b.Queue(veto))
		d := h.waitBroadcasts(1)[0]
		require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
		_, errs := outcomes(t, p.lines())
		require.Equal(t, map[string]string{veto.ID.String(): "sink transaction rejected: " + d.hash() + " code 37: " + log}, errs)

		for range 3 {
			h.tick()
		}
		require.Len(t, h.node.Broadcasts(), 1, "no ordered retry")
	})

	t.Run("refused in simulation", func(t *testing.T) {
		h := newHarness(t, setup{change: func(sc *simnode.Scenario) { sc.Akash.UnorderedDisabled = true }})
		p := h.start(h.options(), h.signer, nil)
		veto := h.plan(h.snapshot(), firstVeto)

		require.True(t, p.b.Queue(veto))
		require.Eventually(t, func() bool { return p.count() > 0 }, 5*time.Second, time.Millisecond)
		_, errs := outcomes(t, p.lines())
		require.Contains(t, errs[veto.ID.String()], "unordered transactions are not enabled")
		require.Empty(t, h.node.Broadcasts())
	})
}

func TestShutdownReleasesQueuedSilently(t *testing.T) {
	// prepare leaves a sent veto, a queued vote and a deferred vote.
	prepare := func(t *testing.T) (*harness, *proc, policy.Planned) {
		h := newHarness(t, setup{})
		p := h.start(h.options(), h.signer, nil)
		st := h.snapshot()
		veto := h.plan(st, firstVeto)

		require.True(t, p.b.Queue(veto))
		h.waitBroadcasts(1)
		h.node.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultNonceTaken}}
		})
		require.True(t, p.b.Queue(h.plan(st, 1)))
		p.b.Flush()
		require.Eventually(t, func() bool { return len(p.deferred()) == 1 }, 5*time.Second, time.Millisecond)
		require.True(t, p.b.Queue(h.plan(st, 2)))

		p.cancel()
		require.Eventually(t, func() bool {
			queued := 0
			p.state(func(s *ownerState) {
				for _, e := range s.entries {
					queued += len(e.queued)
				}
				queued += len(s.deferred)
			})
			return queued == 0
		}, 5*time.Second, time.Millisecond, "queued and deferred messages are released")
		require.False(t, p.b.Queue(h.plan(st, 3)), "nothing is queued after shutdown")
		require.Empty(t, p.lines(), "released without events")

		return h, p, veto
	}

	t.Run("a sent transaction resolves within the grace", func(t *testing.T) {
		h, p, veto := prepare(t)

		h.node.CommitBlock()
		finished := false
		for i := 0; i < 20 && !finished; i++ {
			h.clock.Advance(time.Second)
			select {
			case <-p.done:
				finished = true
			case <-time.After(20 * time.Millisecond):
			}
		}
		require.True(t, finished, "Run returns once the sent transaction resolves")
		hashes, errs := outcomes(t, p.lines())
		require.Contains(t, hashes, veto.ID.String())
		require.Empty(t, errs)
		require.Len(t, h.node.Broadcasts(), 2, "nothing new was signed")
	})

	t.Run("the grace ends", func(t *testing.T) {
		h, p, _ := prepare(t)
		h.node.Halt()

		h.clock.Advance(29 * time.Second)
		select {
		case <-p.done:
			t.Fatal("Run returned before the grace ended")
		case <-time.After(50 * time.Millisecond):
		}

		h.clock.Advance(time.Second)
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return at the end of the grace")
		}
		require.Empty(t, p.lines())
	})
}
