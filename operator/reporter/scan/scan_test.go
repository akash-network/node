package scan_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/observe"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/scan"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

// start is a whole second, so block times and freshness bounds are exact.
var start = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

const (
	maxAge   = 60 * time.Second
	slotLead = uint64(600)
	// head is the latest height of the named scenarios.
	head = int64(100)

	pathState    = "/akash.sink.v1.Query/State"
	pathTransfer = "/akash.sink.v1.Query/Transfer"
	pathDisputes = "/akash.sink.v1.Query/Disputes"
	pathList     = "/akash.sink.v1.Query/Transfers"

	alertChanged = "light-client-changed"
	// deploymentKeys is the key count of a getMultipleAccounts call without status accounts.
	deploymentKeys = 4
	chunkSize      = 96
)

var (
	recipient = solana.PublicKeyFromBytes(bytes.Repeat([]byte{7}, 32))
	other     = solana.PublicKeyFromBytes(bytes.Repeat([]byte{5}, 32))
)

// output collects the event lines; it is safe to read while the writer runs.
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

// recorder is a Queuer that keeps every planned message; onQueue, when set, runs before each is recorded.
type recorder struct {
	mu      sync.Mutex
	planned []policy.Planned
	onQueue func(policy.Planned)
}

func (r *recorder) Queue(p policy.Planned) bool {
	if r.onQueue != nil {
		r.onQueue(p)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.planned = append(r.planned, p)

	return true
}

func (r *recorder) all() []policy.Planned {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]policy.Planned(nil), r.planned...)
}

// setup configures a harness.
type setup struct {
	opts  scan.Options
	pin   *solana.PublicKey
	queue *recorder
	// solana, when set, sees each Solana request first, with its method and getMultipleAccounts key count; returning
	// true fails the request with HTTP 503.
	solana func(r *http.Request, method string, keys int) bool
}

type harness struct {
	t        *testing.T
	node     *simnode.Node
	clock    *clock.Fake
	akash    *akash.Client
	out      *output
	ev       *events.Writer
	scanner  *scan.Scanner
	syncs    int
	consumed int
}

func newHarness(t *testing.T, sc simnode.Scenario, s setup) *harness {
	t.Helper()

	fake := clock.NewFake(start)
	n, err := simnode.Start(t.Context(), sc, simnode.Options{Clock: fake})
	require.NoError(t, err)
	t.Cleanup(n.Close)

	a, err := akash.Dial(t.Context(), akash.Options{
		Node:           n.CometURL,
		GRPCAddr:       n.GRPCAddr,
		ChainID:        simnode.ChainID,
		MaxAge:         maxAge,
		RequestTimeout: 5 * time.Second,
		Concurrency:    16,
		Clock:          fake,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	solanaURL := n.SolanaURL
	if s.solana != nil {
		solanaURL = proxy(t, n.SolanaURL, s.solana)
	}
	rpc := aktsolana.NewRPC(aktsolana.RPCOptions{URL: solanaURL, Timeout: 5 * time.Second, Concurrency: 8, BlockConcurrency: 2})

	h := &harness{t: t, node: n, clock: fake, akash: a, out: &output{}}
	h.ev = events.NewWriter(h.out)
	t.Cleanup(func() { _ = h.ev.Close() })

	// The observer's callback reaches the scanner built after it, as the CLI wires them.
	var scanner *scan.Scanner
	o := observe.New(rpc, aktsolana.NewEvidenceIndex(rpc, simnode.ProgramID, 256, 64<<20), observe.Options{
		GenesisHash:    simnode.GenesisHash,
		ProgramID:      simnode.ProgramID,
		Mint:           simnode.MintAddress,
		LightClientPin: s.pin,
		ChainID:        simnode.ChainID,
		MaxAge:         maxAge,
		SlotLead:       slotLead,
		Clock:          fake,
	}, func(c observe.Change) { scanner.OnChange(c) })

	opts := s.opts
	if opts.Reporter == "" {
		opts.Reporter = simnode.Reporter
	}
	if opts.Workers == 0 {
		opts.Workers = 1
	}
	if opts.PageSize == 0 {
		opts.PageSize = 500
	}
	if opts.AuditPerScan == 0 {
		opts.AuditPerScan = 10
	}
	opts.Clock = fake

	var q scan.Queuer
	if s.queue != nil {
		q = s.queue
		opts.Broadcast = true
	}
	scanner = scan.New(a, o, h.ev, opts, q)
	h.scanner = scanner

	return h
}

// proxy forwards Solana requests to target once before has seen them.
func proxy(t *testing.T, target string, before func(r *http.Request, method string, keys int) bool) string {
	t.Helper()

	u, err := url.Parse(target)
	require.NoError(t, err)
	forward := httputil.NewSingleHostReverseProxy(u)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		var keys []string
		if json.Unmarshal(body, &req) == nil && req.Method == "getMultipleAccounts" && len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params[0], &keys)
		}

		if before(r, req.Method, len(keys)) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

// chunkCalls calls f with the ordinal of each getMultipleAccounts call that carries status accounts.
func chunkCalls(f func(r *http.Request, call int) bool) func(*http.Request, string, int) bool {
	var calls atomic.Int64

	return func(r *http.Request, method string, keys int) bool {
		if method != "getMultipleAccounts" || keys <= deploymentKeys {
			return false
		}

		return f(r, int(calls.Add(1)))
	}
}

// changeBridgeAt names other as the light client just before the call-th chunk snapshot reads the bridge.
func changeBridgeAt(call int, n *atomic.Pointer[simnode.Node]) func(*http.Request, string, int) bool {
	return chunkCalls(func(_ *http.Request, c int) bool {
		if c == call {
			setBridge(n.Load(), other)
		}

		return false
	})
}

// lines returns the event lines written since the previous call. It emits a marker and waits for it, so every event
// emitted before the call is included.
func (h *harness) lines() []string {
	h.t.Helper()

	h.syncs++
	marker := `{"sync":` + strconv.Itoa(h.syncs) + `}`
	h.ev.Emit(json.RawMessage(marker))

	var all []string
	require.Eventually(h.t, func() bool {
		all = strings.Split(strings.TrimSuffix(h.out.text(), "\n"), "\n")
		return all[len(all)-1] == marker
	}, 5*time.Second, time.Millisecond)

	var out []string
	for _, l := range all[h.consumed:] {
		if !strings.HasPrefix(l, `{"sync":`) {
			out = append(out, l)
		}
	}
	h.consumed = len(all)

	return out
}

func decode(t *testing.T, line string) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &m), line)

	return m
}

// reviews returns the IDs of the review events among lines, in order.
func reviews(t *testing.T, lines []string) []string {
	t.Helper()

	var out []string
	for _, l := range lines {
		m := decode(t, l)
		if _, ok := m["observed"]; ok {
			out = append(out, m["id"].(string))
		}
	}

	return out
}

// errorLines returns the {id, error} events among lines by ID.
func errorLines(t *testing.T, lines []string) map[string]string {
	t.Helper()

	out := map[string]string{}
	for _, l := range lines {
		m := decode(t, l)
		id, isTransfer := m["id"].(string)
		text, isError := m["error"].(string)
		if isTransfer && isError {
			out[id] = text
		}
	}

	return out
}

func summaryOf(t *testing.T, line string) events.Summary {
	t.Helper()

	require.True(t, isSummary(t, line), line)

	var s events.Summary
	require.NoError(t, json.Unmarshal([]byte(line), &s))

	return s
}

func isSummary(t *testing.T, line string) bool {
	t.Helper()

	_, ok := decode(t, line)["scanned"]

	return ok
}

func summary(scanned, audited, failures int) events.Summary {
	return events.Summary{
		Scanned: scanned, Audited: audited, Index: "canonical", Failures: failures,
		LightClientProgram: simnode.LightClient.String(),
	}
}

// scenario is the clean scenario serving transfers instead of its own.
func scenario(transfers ...sinkv1.TransferState) simnode.Scenario {
	sc := simnode.Scenarios()["clean"]()
	sc.Akash.Transfers = nil
	for _, t := range transfers {
		sc.Akash.Transfers = append(sc.Akash.Transfers, simnode.Transfer{Transfer: t})
	}

	return sc
}

func newID(t *testing.T, fill byte, height int64, sequence uint64) sinkid.ID {
	t.Helper()

	raw := bytes.Repeat([]byte{fill}, 20)
	owner, err := bech32.ConvertAndEncode("akash", raw)
	require.NoError(t, err)

	return sinkid.ID{Owner: owner, OwnerBytes: raw, Height: height, Sequence: sequence}
}

// transfer is a valid transfer under the named scenarios' policy.
func transfer(id sinkid.ID, st sinkv1.Status, hold sinkv1.Hold) sinkv1.TransferState {
	return sinkv1.TransferState{
		ID:         id.Proto(),
		Data:       sinkv1.RecordData{Dest: recipient.String(), Amount: sdk.NewInt64Coin("uakt", 1_000)},
		Status:     st,
		Settlement: sinkv1.SettlementData{Round: 1, Policy: simnode.Scenarios()["clean"]().Akash.Policies[0], Hold: hold},
	}
}

// pending returns n transfer-pending transfers of one owner, in record-key and chronological order.
func pending(t *testing.T, n int) ([]sinkid.ID, []sinkv1.TransferState) {
	t.Helper()

	ids := make([]sinkid.ID, n)
	transfers := make([]sinkv1.TransferState, n)
	for i := range n {
		ids[i] = newID(t, 9, 5, uint64(i+1)) //nolint:gosec // small
		transfers[i] = transfer(ids[i], sinkv1.StatusTransferPending, sinkv1.HoldNone)
	}

	return ids, transfers
}

func texts(ids []sinkid.ID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}

	return out
}

func idsOf(t *testing.T, sc simnode.Scenario) []sinkid.ID {
	t.Helper()

	ids := make([]sinkid.ID, len(sc.Akash.Transfers))
	for i, tr := range sc.Akash.Transfers {
		id, err := sinkid.FromProto(tr.Transfer.ID)
		require.NoError(t, err)
		ids[i] = id
	}

	return ids
}

func statusAccount(st simnode.TransferStatus) simnode.Account {
	return simnode.Account{Owner: simnode.ProgramID, Lamports: 1, Data: simnode.EncodeTransferStatus(st)}
}

// resolution serves the status st and its resolution as the only transaction of its slot's block.
func resolution(s *simnode.Solana, st simnode.TransferStatus, sigFill byte) {
	address := aktsolana.StatusAddress(simnode.ProgramID, st.ID)
	s.Accounts[address] = statusAccount(st)
	s.BlockTimes[st.ResolvedAtSlot] = -time.Minute
	s.Blocks[st.ResolvedAtSlot] = simnode.Block{Transactions: []simnode.Transaction{{
		Signatures:  []string{solana.SignatureFromBytes(bytes.Repeat([]byte{sigFill}, 64)).String()},
		Header:      simnode.Header{NumRequiredSignatures: 1, NumReadonlyUnsignedAccounts: 1},
		AccountKeys: []solana.PublicKey{recipient, address, simnode.ProgramID},
		Logs: []string{
			"Program " + simnode.ProgramID.String() + " invoke [1]",
			simnode.TransferResolvedLog(st),
			"Program " + simnode.ProgramID.String() + " success",
		},
	}}}
}

func setTransfer(n *simnode.Node, id sinkid.ID, change func(*sinkv1.TransferState)) {
	n.Mutate(func(sc *simnode.Scenario) {
		for i := range sc.Akash.Transfers {
			if got, err := sinkid.FromProto(sc.Akash.Transfers[i].Transfer.ID); err == nil && sinkid.Compare(got, id) == 0 {
				change(&sc.Akash.Transfers[i].Transfer)
			}
		}
	})
}

func setBridge(n *simnode.Node, lightClient solana.PublicKey) {
	n.Mutate(func(sc *simnode.Scenario) {
		sc.Solana.Accounts[aktsolana.PDA(simnode.ProgramID, []byte("bridge"))] = simnode.Account{
			Owner: simnode.ProgramID, Lamports: 1, Data: simnode.EncodeBridgeConfig(simnode.BridgeConfig{
				LightClientProgram: lightClient, ChainID: simnode.ChainID,
				PerTransferMax: 1_000_000_000_000, DailyMintCap: 1_000_000_000_000,
			}),
		}
	})
}

func TestScanFromTS(t *testing.T) {
	t.Run("all held terminal disputes are reviewed first and reread before decisions", func(t *testing.T) {
		base := newID(t, 1, 10, 1)
		held := newID(t, 2, 11, 1)
		h := newHarness(t, scenario(
			transfer(base, sinkv1.StatusTransferPending, sinkv1.HoldNone),
			transfer(held, sinkv1.StatusRefunded, sinkv1.HoldConflict),
		), setup{})

		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))

		lines := h.lines()
		require.Equal(t, []string{held.String(), base.String()}, reviews(t, lines))
		require.Equal(t, 2, h.node.Calls(pathTransfer), "each transfer is reread at the chunk height")
		require.Equal(t, summary(2, 2, 0), summaryOf(t, lines[len(lines)-1]))
	})

	t.Run("pruned queries and light-client pin mismatches stop the scan without fallback", func(t *testing.T) {
		id := newID(t, 1, 10, 1)
		h := newHarness(t, scenario(transfer(id, sinkv1.StatusTransferPending, sinkv1.HoldNone)), setup{})

		// The tracker keeps its snapshot of height 100 while the node prunes that height.
		st, err := h.akash.Latest(t.Context())
		require.NoError(t, err)
		require.Equal(t, head, st.Height)
		h.node.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.Height = head + 1
			sc.Akash.BlockTimes = map[int64]time.Duration{head + 1: -time.Second}
		})

		require.Equal(t, 1, h.scanner.Pass(t.Context(), nil))
		lines := h.lines()
		require.Len(t, lines, 1)
		require.Contains(t, decode(t, lines[0])["error"], "version does not exist")
		require.Zero(t, h.node.Calls(pathTransfer))

		g := newHarness(t, scenario(transfer(id, sinkv1.StatusTransferPending, sinkv1.HoldNone)), setup{pin: &other})

		require.Equal(t, 1, g.scanner.Pass(t.Context(), nil))
		lines = g.lines()
		require.Len(t, lines, 2)
		require.JSONEq(t, `{"alert":"light-client-mismatch","pinned":"`+other.String()+
			`","current":"`+simnode.LightClient.String()+`"}`, lines[0])
		require.Contains(t, decode(t, lines[1])["error"], "differs from pinned")
		require.Zero(t, g.node.Calls(pathTransfer))
	})

	t.Run("every open transfer past the old bounds is reviewed, disputes and refunds first", func(t *testing.T) {
		const held, refunds, ordered, waiting = 501, 501, 501, 8_498

		var (
			sequence uint64
			order    []string
			all      []sinkv1.TransferState
		)
		row := func(st sinkv1.Status, hold sinkv1.Hold) {
			sequence++
			id := newID(t, 9, 5, sequence)
			order = append(order, id.String())
			all = append(all, transfer(id, st, hold))
		}
		disputed := []sinkv1.Status{sinkv1.StatusRefundPending, sinkv1.StatusTransferPending, sinkv1.StatusMinted}
		for i := range held {
			hold := sinkv1.HoldVeto
			if i%3 == 2 {
				hold = sinkv1.HoldConflict
			}
			row(disputed[i%3], hold)
		}
		for range refunds {
			row(sinkv1.StatusRefundPending, sinkv1.HoldNone)
		}
		for range ordered {
			row(sinkv1.StatusVoided, sinkv1.HoldNone)
		}
		for range waiting {
			row(sinkv1.StatusTransferPending, sinkv1.HoldNone)
		}
		require.Len(t, all, 10_001, "one more than the old maximum bound")

		h := newHarness(t, scenario(all...), setup{})
		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))

		lines := h.lines()
		require.Equal(t, order, reviews(t, lines), "each open transfer once, urgent first")
		require.Greater(t, h.node.Calls(pathDisputes), 1, "disputes read across pages")
		require.Greater(t, h.node.Calls(pathList), 3+1, "status buckets read across pages")
		require.Equal(t, summary(10_001, 10, 0), summaryOf(t, lines[len(lines)-1]))
	})

	t.Run("an audit query failure still reviews open transfers and retries its window", func(t *testing.T) {
		waiting := newID(t, 1, 10, 1)
		held := newID(t, 2, 11, 1)
		terminal := newID(t, 3, 12, 1)
		terminalRow := simnode.Transfer{Transfer: transfer(terminal, sinkv1.StatusMinted, sinkv1.HoldNone)}

		h := newHarness(t, scenario(
			transfer(waiting, sinkv1.StatusTransferPending, sinkv1.HoldNone),
			transfer(held, sinkv1.StatusRefunded, sinkv1.HoldConflict),
			terminalRow.Transfer,
		), setup{opts: scan.Options{AuditPerScan: 2}})

		// The first history page holds waiting and held, with a cursor to terminal.
		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		lines := h.lines()
		require.Equal(t, []string{held.String(), waiting.String()}, reviews(t, lines))
		require.Equal(t, summary(2, 2, 0), summaryOf(t, lines[len(lines)-1]))

		// The checkpoint disappears: the audit fails, open work is still reviewed, and the cursor is kept.
		h.node.Mutate(func(sc *simnode.Scenario) { sc.Akash.Transfers = sc.Akash.Transfers[:2] })
		require.Equal(t, 1, h.scanner.Pass(t.Context(), nil))
		lines = h.lines()
		require.JSONEq(t, `{"alert":"audit-unavailable","error":"chronological checkpoint disappeared"}`, lines[0])
		require.Equal(t, []string{held.String(), waiting.String()}, reviews(t, lines), "open work still reviewed")
		require.Equal(t, summary(2, 0, 1), summaryOf(t, lines[len(lines)-1]))

		// The window is retried from the kept cursor.
		h.node.Mutate(func(sc *simnode.Scenario) { sc.Akash.Transfers = append(sc.Akash.Transfers, terminalRow) })
		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		lines = h.lines()
		require.Equal(t, []string{held.String(), waiting.String(), terminal.String()}, reviews(t, lines))
		require.Equal(t, true, decode(t, lines[2])["audit"])
		require.Equal(t, summary(2, 1, 0), summaryOf(t, lines[len(lines)-1]))

		// That page had no more, so its tail cursor is dropped and the audit starts over.
		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		lines = h.lines()
		require.Equal(t, []string{held.String(), waiting.String()}, reviews(t, lines))
		require.Equal(t, summary(2, 2, 0), summaryOf(t, lines[len(lines)-1]))
	})

	t.Run("broadcast scans do not pay for bare void orders, frozen same-outcome votes or existing conflicts", func(t *testing.T) {
		id := newID(t, 1, 10, 1)
		order := transfer(id, sinkv1.StatusVoided, sinkv1.HoldNone)
		order.Settlement.VoidAuthorization = &sinkv1.VoidAuthorization{RequestedHeight: 1, RequestedRound: 1}
		q := &recorder{}
		h := newHarness(t, scenario(order), setup{queue: q})

		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		require.Empty(t, q.all())
		review := decode(t, h.lines()[0])
		require.Equal(t, id.String(), review["id"])
		require.Equal(t, "broadcast", review["mode"])
		require.Equal(t, []any{}, review["alerts"])

		// Every member, this reporter included, already voted the finalized void.
		st, err := h.akash.Latest(t.Context())
		require.NoError(t, err)
		observation := sinkv1.Observation{Outcome: sinkv1.OutcomeVoided, Slot: 900, TransactionSignature: bytes.Repeat([]byte{1}, 64)}
		setTransfer(h.node, id, func(tr *sinkv1.TransferState) {
			tr.Status = sinkv1.StatusRefundPending
			tr.Settlement.ChallengeEnd = st.Time.Unix() + 100
			tr.Settlement.Votes = nil
			for _, m := range st.Policy.Members {
				tr.Settlement.Votes = append(tr.Settlement.Votes, sinkv1.Vote{Reporter: m, PolicyVersion: 1, Observation: observation})
			}
		})
		voided := simnode.VoidGovernance
		h.node.Mutate(func(sc *simnode.Scenario) {
			sc.Solana.Accounts[aktsolana.StatusAddress(simnode.ProgramID, id)] = statusAccount(simnode.TransferStatus{
				ID: id, ProofHeight: 11, ResolvedAtSlot: 900, Outcome: simnode.Outcome{Voided: &voided},
			})
		})

		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		require.Empty(t, q.all())
		require.Empty(t, errorLines(t, h.lines()))

		h.node.Mutate(func(sc *simnode.Scenario) {
			resolution(&sc.Solana, simnode.TransferStatus{
				ID: id, ProofHeight: 11, ResolvedAtSlot: 901,
				Outcome: simnode.Outcome{Minted: &simnode.Minted{Amount: 1_000, Recipient: recipient}},
			}, 4)
		})
		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		planned := q.all()
		require.Len(t, planned, 1, "first opposite witness is actionable")
		vote, ok := planned[0].Msg.(*sinkv1.MsgReportOutcome)
		require.True(t, ok)
		require.Equal(t, sinkv1.OutcomeMinted, vote.Observation.Outcome)

		setTransfer(h.node, id, func(tr *sinkv1.TransferState) { tr.Settlement.Hold = sinkv1.HoldConflict })
		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
		require.Len(t, q.all(), 1, "conflict never causes endless paid reports")
	})
}

func TestOrderingGuarantees(t *testing.T) {
	ids, transfers := pending(t, 150)

	var n atomic.Pointer[simnode.Node]
	h := newHarness(t, scenario(transfers...), setup{solana: changeBridgeAt(2, &n)})
	n.Store(h.node)

	require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
	lines := h.lines()

	changed := -1
	position := map[string]int{}
	for i, l := range lines {
		m := decode(t, l)
		if m["alert"] == alertChanged {
			require.Equal(t, -1, changed, "one alert")
			changed = i
			require.JSONEq(t, `{"alert":"light-client-changed","previous":"`+simnode.LightClient.String()+
				`","current":"`+other.String()+`","slot":"1000"}`, l)
		}
		if id, ok := m["id"].(string); ok {
			position[id] = i
		}
	}
	require.Positive(t, changed)
	for i, id := range ids {
		if i < chunkSize {
			require.Less(t, position[id.String()], changed, "the first chunk is decided under the previous client")
		} else {
			require.Greater(t, position[id.String()], changed, "the second chunk is decided under the new client")
		}
	}

	require.Equal(t, summary(150, 10, 0), summaryOf(t, lines[len(lines)-1]), "the summary is the last line")
}

func TestSettleBeforeSummary(t *testing.T) {
	h := newHarness(t, simnode.Scenarios()["clean"](), setup{})

	calls := 0
	failures := h.scanner.Pass(t.Context(), func() int {
		calls++
		h.ev.Emit(events.TransferError{ID: "settled", Error: "sink transaction rejected"})

		return 3
	})
	require.Equal(t, 1, calls)
	require.Equal(t, 3, failures)

	lines := h.lines()
	require.Len(t, reviews(t, lines), 5)
	require.JSONEq(t, `{"id":"settled","error":"sink transaction rejected"}`, lines[len(lines)-2])
	require.Equal(t, summary(5, 5, 3), summaryOf(t, lines[len(lines)-1]))
}

func TestPinMismatchStopsAfterRunningChunks(t *testing.T) {
	pin := simnode.LightClient

	// end checks the pass's last lines: the alert and the error, no summary, and only reviews written before them.
	end := func(t *testing.T, h *harness, settle *int, failures int) []string {
		t.Helper()

		require.Equal(t, 1, *settle, "settle still runs once")
		require.Equal(t, 1, failures)

		lines := h.lines()
		require.GreaterOrEqual(t, len(lines), 2)
		require.JSONEq(t, `{"alert":"light-client-mismatch","pinned":"`+pin.String()+`","current":"`+other.String()+`"}`, lines[len(lines)-2])
		require.Contains(t, decode(t, lines[len(lines)-1])["error"], "differs from pinned")
		for _, l := range lines {
			require.False(t, isSummary(t, l), "no summary")
		}
		require.Empty(t, errorLines(t, lines), "cancelled reviews write nothing")

		return lines
	}

	t.Run("a held chunk snapshot", func(t *testing.T) {
		ids, transfers := pending(t, 4*chunkSize)

		// Two workers: the first chunk snapshot passes and its chunk is reviewed; the second is held in flight; the
		// third, taken by the worker that finished the first, sees a new light client and must cancel the held one.
		var n atomic.Pointer[simnode.Node]
		h := newHarness(t, scenario(transfers...), setup{
			pin:  &pin,
			opts: scan.Options{Workers: 2},
			solana: chunkCalls(func(r *http.Request, call int) bool {
				switch call {
				case 2:
					select {
					case <-r.Context().Done():
					case <-time.After(5 * time.Second):
					}
				case 3:
					setBridge(n.Load(), other)
				}

				return false
			}),
		})
		n.Store(h.node)

		settle := 0
		failures := h.scanner.Pass(t.Context(), func() int { settle++; return 0 })

		lines := end(t, h, &settle, failures)
		require.Len(t, lines, chunkSize+2)
		reviewed := reviews(t, lines)
		require.Contains(t, [][]string{texts(ids[:chunkSize]), texts(ids[chunkSize : 2*chunkSize])}, reviewed,
			"the reviews of the chunk snapshotted before the change stand")
	})

	t.Run("a held evidence lookup", func(t *testing.T) {
		ids, transfers := pending(t, 2*chunkSize)
		sc := scenario()
		voided := simnode.VoidGovernance
		for i, id := range ids {
			// One bucket keeps the chunks in sequence order; the first transfer of each needs its resolution block.
			transfers[i].Status = sinkv1.StatusVoided
			transfers[i].Settlement.VoidAuthorization = &sinkv1.VoidAuthorization{RequestedHeight: 1, RequestedRound: 1}
			if i%chunkSize == 0 {
				resolution(&sc.Solana, simnode.TransferStatus{
					ID: id, ProofHeight: 6, ResolvedAtSlot: 900 + uint64(i), Outcome: simnode.Outcome{Voided: &voided}, //nolint:gosec // small
				}, byte(i%250+1)) //nolint:gosec // small
			}
			sc.Akash.Transfers = append(sc.Akash.Transfers, simnode.Transfer{Transfer: transfers[i]})
		}

		// The chunk whose snapshot passes holds its block fetch; the other snapshot then sees a new light client, and
		// the mismatch must cancel the fetch.
		var (
			n         atomic.Pointer[simnode.Node]
			fetching  = make(chan struct{})
			fetchOnce sync.Once
		)
		chunk := chunkCalls(func(_ *http.Request, call int) bool {
			if call == 2 {
				select {
				case <-fetching:
				case <-time.After(5 * time.Second):
				}
				setBridge(n.Load(), other)
			}

			return false
		})
		h := newHarness(t, sc, setup{
			pin:  &pin,
			opts: scan.Options{Workers: 2},
			solana: func(r *http.Request, method string, keys int) bool {
				if method == "getBlock" {
					fetchOnce.Do(func() { close(fetching) })
					select {
					case <-r.Context().Done():
					case <-time.After(5 * time.Second):
					}
				}

				return chunk(r, method, keys)
			},
		})
		n.Store(h.node)

		settle := 0
		failures := h.scanner.Pass(t.Context(), func() int { settle++; return 0 })

		lines := end(t, h, &settle, failures)
		require.Len(t, lines, 2, "the held review is cancelled before its event")
	})
}

func TestChunkErrorsArePerTransfer(t *testing.T) {
	ids, transfers := pending(t, 150)
	h := newHarness(t, scenario(transfers...), setup{solana: chunkCalls(func(_ *http.Request, call int) bool { return call == 1 })})

	require.Equal(t, chunkSize, h.scanner.Pass(t.Context(), nil))

	lines := h.lines()
	failed := errorLines(t, lines)
	require.Len(t, failed, chunkSize)
	for _, id := range ids[:chunkSize] {
		require.Equal(t, "Solana node is temporarily unavailable", failed[id.String()])
	}
	require.Equal(t, texts(ids[chunkSize:]), reviews(t, lines), "the next chunk is reviewed")
	require.Equal(t, summary(150, 10, chunkSize), summaryOf(t, lines[len(lines)-1]))
}

func TestAuditEarlyExitEvent(t *testing.T) {
	id := newID(t, 1, 10, 1)
	sc := scenario(transfer(id, sinkv1.StatusMinted, sinkv1.HoldNone))
	sc.Solana.Accounts[aktsolana.StatusAddress(simnode.ProgramID, id)] = statusAccount(simnode.TransferStatus{
		ID: id, ProofHeight: 11, ResolvedAtSlot: 900,
		Outcome: simnode.Outcome{Minted: &simnode.Minted{Amount: 1_000, Recipient: recipient}},
	})
	h := newHarness(t, sc, setup{})

	require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))
	lines := h.lines()
	require.Len(t, lines, 2)
	require.Equal(t, `{"id":"`+id.String()+`","audit":true,"observed":"resolved","messages":[],"alerts":[]}`, lines[0])
	require.Equal(t, summary(0, 1, 0), summaryOf(t, lines[1]))
	require.Zero(t, h.node.Calls("getBlock"), "the early exit needs no evidence")
}

func TestCancelledPassReportsListedCount(t *testing.T) {
	sc := simnode.Scenarios()["votes"]()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	q := &recorder{onQueue: func(policy.Planned) { cancel() }}
	h := newHarness(t, sc, setup{queue: q})

	calls := 0
	require.Equal(t, 0, h.scanner.Pass(ctx, func() int { calls++; return 0 }))
	require.Equal(t, 1, calls)

	lines := h.lines()
	require.Len(t, reviews(t, lines), 1, "no review starts after the cancel")
	require.Len(t, q.all(), 1)
	require.Equal(t, len(sc.Akash.Transfers), summaryOf(t, lines[len(lines)-1]).Scanned)

	// Cancelled before it starts, a pass still lists, then reviews nothing.
	require.Equal(t, 0, h.scanner.Pass(ctx, nil))
	lines = h.lines()
	require.Len(t, lines, 1)
	require.Equal(t, len(sc.Akash.Transfers), summaryOf(t, lines[0]).Scanned)
	require.Len(t, q.all(), 1)
}

func TestPlannedCarriesSnapshot(t *testing.T) {
	sc := simnode.Scenarios()["votes"]()
	q := &recorder{}
	h := newHarness(t, sc, setup{queue: q, opts: scan.Options{Workers: 4}})
	// The writer keeps emission order, so a marker written at queue time shows what was written before.
	q.onQueue = func(p policy.Planned) { h.ev.Emit(events.TxHash{ID: p.ID.String(), TransactionHash: "queued"}) }

	require.Equal(t, 0, h.scanner.Pass(t.Context(), nil))

	st, err := h.akash.Latest(t.Context())
	require.NoError(t, err)
	planned := q.all()
	require.Len(t, planned, len(sc.Akash.Transfers))
	for _, p := range planned {
		require.Equal(t, st.Height, p.PlanHeight)
		require.True(t, st.Time.Equal(p.PlanTime))
		require.Equal(t, policy.KeyOf(p.ID, p.Msg), p.Key)
		vote, ok := p.Msg.(*sinkv1.MsgReportOutcome)
		require.True(t, ok)
		require.Equal(t, p.ID.Proto(), vote.ID)
	}

	lines := h.lines()
	require.Len(t, reviews(t, lines), len(sc.Akash.Transfers))
	reviewed := map[string]bool{}
	for _, l := range lines {
		m := decode(t, l)
		id, _ := m["id"].(string)
		if _, ok := m["observed"]; ok {
			reviewed[id] = true
		}
		if m["transactionHash"] == "queued" {
			require.True(t, reviewed[id], "the review event of %s precedes its queued message", id)
		}
	}
}

func TestReviewIDs(t *testing.T) {
	sc := simnode.Scenarios()["votes"]()
	ids := idsOf(t, sc)
	q := &recorder{}
	h := newHarness(t, sc, setup{queue: q})

	setTransfer(h.node, ids[2], func(tr *sinkv1.TransferState) { tr.Status = sinkv1.StatusMinted })
	states := h.node.Calls(pathState)

	require.NoError(t, h.scanner.Review(t.Context(), ids[:3], 0))
	require.Equal(t, states+1, h.node.Calls(pathState), "one fresh snapshot")

	lines := h.lines()
	require.Len(t, lines, 2)
	require.Equal(t, texts(ids[:2]), reviews(t, lines), "a transfer no longer open writes no event")

	st, err := h.akash.Latest(t.Context())
	require.NoError(t, err)
	planned := q.all()
	require.Len(t, planned, 2)
	for i, p := range planned {
		require.Equal(t, ids[i].String(), p.ID.String())
		require.Equal(t, st.Height, p.PlanHeight)
		require.True(t, st.Time.Equal(p.PlanTime))
	}
}

func TestReviewRefundHead(t *testing.T) {
	sc := simnode.Scenarios()["votes"]()
	ids := idsOf(t, sc)
	h := newHarness(t, sc, setup{queue: &recorder{}})

	require.NoError(t, h.scanner.ReviewRefundHead(t.Context(), head))
	require.Empty(t, h.lines(), "no refund head, nothing to review")

	st, err := h.akash.Latest(t.Context())
	require.NoError(t, err)
	setTransfer(h.node, ids[4], func(tr *sinkv1.TransferState) {
		tr.Status = sinkv1.StatusRefundPending
		tr.Settlement.ChallengeEnd = st.Time.Unix() + 100
	})
	h.clock.Advance(time.Second)

	require.NoError(t, h.scanner.ReviewRefundHead(t.Context(), head))
	require.Equal(t, []string{ids[4].String()}, reviews(t, h.lines()))
}

func TestReviewWaitsForHeight(t *testing.T) {
	sc := simnode.Scenarios()["votes"]()
	ids := idsOf(t, sc)
	q := &recorder{}
	h := newHarness(t, sc, setup{queue: q})

	h.node.SetLag(1)
	states := h.node.Calls(pathState)
	done := make(chan error, 1)
	go func() { done <- h.scanner.Review(t.Context(), ids[:1], head) }()

	require.Eventually(t, func() bool { return h.node.Calls(pathState) > states }, 5*time.Second, time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("Review returned before the lag cleared: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	require.Empty(t, h.lines(), "no review until the lag clears")

	// Each retry waits a second of the fake clock; advance until one sees height 100.
	h.node.SetLag(0)
	var err error
	finished := false
	for i := 0; i < 10 && !finished; i++ {
		h.clock.Advance(time.Second)
		select {
		case err = <-done:
			finished = true
		case <-time.After(200 * time.Millisecond):
		}
	}
	require.True(t, finished, "Review returns once the lag clears")
	require.NoError(t, err)
	require.Equal(t, []string{ids[0].String()}, reviews(t, h.lines()))
	planned := q.all()
	require.Len(t, planned, 1)
	require.Equal(t, head, planned[0].PlanHeight)

	// A context that ends first returns its error and writes nothing.
	h.node.SetLag(1)
	h.clock.Advance(time.Second)
	ctx, cancel := context.WithCancel(t.Context())
	states = h.node.Calls(pathState)
	go func() { done <- h.scanner.Review(ctx, ids[1:2], head) }()
	require.Eventually(t, func() bool { return h.node.Calls(pathState) > states }, 5*time.Second, time.Millisecond)
	cancel()
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Review did not return after its context ended")
	}
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, h.lines())
	require.Len(t, q.all(), 1)
}
