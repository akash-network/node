package scan_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/events"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/observe"
	"pkg.akt.dev/node/v2/operator/reporter/scan"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

// goldenDir holds the events the TS reporter wrote for each simnode scenario (contracts/solana/reporter/goldens.ts).
const goldenDir = "../testdata/goldens"

// The CLI defaults (spec §8.3) the goldens run the Go reporter with; contracts/solana/reporter/goldens.ts runs the
// TS reporter with auditPerScan 100 and the max-age that simnode serve prints.
const (
	goldenTimeout      = 15 * time.Second
	goldenWorkers      = 16
	goldenAkashReads   = 32
	goldenSolanaReads  = 8
	goldenBlockReads   = 2
	goldenPageSize     = 500
	goldenAuditPerScan = 100
	goldenEvidenceSize = 256
	goldenEvidenceMem  = 64 << 20
)

// goldenSlotLead is the default of solana.max-slot-lead: (max-age + 60 s) × 5 slots per second.
var goldenSlotLead = uint64((simnode.ReporterMaxAge + time.Minute) / time.Second * 5) //nolint:gosec // positive

// allowance admits a field that may differ from the golden in one scenario, under an intentional difference of spec
// §5.6. ID is the transfer, or empty for pass-level events. The field must still be present on both sides.
type allowance struct {
	scenario, id, field string
	// difference is the number of the §5.6 intentional difference.
	difference int
}

// allowances are the only accepted differences from the goldens.
var allowances = []allowance{
	// The failed audit query's text is the transport's: TS reads the history through abci_query, Go through gRPC.
	{scenario: "audit-resume", field: "error", difference: 3},
}

func TestGoldens(t *testing.T) {
	scenarios := simnode.Scenarios()

	files, err := filepath.Glob(filepath.Join(goldenDir, "*.jsonl"))
	require.NoError(t, err)
	stored := make([]string, 0, len(files))
	for _, f := range files {
		stored = append(stored, strings.TrimSuffix(filepath.Base(f), ".jsonl"))
	}
	require.ElementsMatch(t, simnode.Goldens(), stored, "every golden scenario has exactly one golden")

	for _, name := range simnode.Goldens() {
		t.Run(name, func(t *testing.T) {
			sc := scenarios[name]()
			want := goldenPasses(t, readEvents(t, filepath.Join(goldenDir, name+".jsonl")), sc.PassCount())
			got := runGoldenPasses(t, sc)
			require.Len(t, got, len(want))

			for i := range want {
				comparePass(t, name, i+1, want[i], got[i])
			}
		})
	}
}

func TestGoldensErrorCut(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"one line", "stale Akash node", "stale Akash node"},
		{
			"actual-expected diff",
			"mint amount differs from Akash deposit\n+ actual - expected\n\n+ 1000001n\n- 1000000n\n        ^\n",
			"mint amount differs from Akash deposit",
		},
		{"strict inequality", "custom text\n\n1n !== 2n\n", "custom text"},
		// Any other second line is part of the text, so it fails against a one-line Go error.
		{"other second line", "A\nanything", "A\nanything"},
		{"marker not at the start", "A\nx + actual - expected\n", "A\nx + actual - expected\n"},
		{"blank line without inequality", "A\n\nB\n", "A\n\nB\n"},
		{"inequality not on its own line", "A\n\n1n !== 2n\nmore", "A\n\n1n !== 2n\nmore"},
	} {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, tsMessage(c.text))
		})
	}
}

// event is one decoded event line.
type event = map[string]any

func readEvents(t *testing.T, path string) []event {
	t.Helper()

	raw, err := os.ReadFile(path) //nolint:gosec // a golden under goldenDir
	require.NoError(t, err)

	return decodeEvents(t, path, raw)
}

func decodeEvents(t *testing.T, path string, raw []byte) []event {
	t.Helper()

	var out []event
	s := bufio.NewScanner(bytes.NewReader(raw))
	s.Buffer(nil, 16<<20)
	for s.Scan() {
		if len(s.Bytes()) == 0 {
			continue
		}

		var e event
		require.NoError(t, json.Unmarshal(s.Bytes(), &e), "%s: %s", path, s.Text())
		out = append(out, e)
	}
	require.NoError(t, s.Err())

	return out
}

// terminal reports the event that ends a pass: its summary, or the error without an ID that ends it early.
func terminal(e event) bool {
	if _, ok := e["scanned"]; ok {
		return true
	}

	_, isError := e["error"]
	_, hasID := e["id"]
	_, isAlert := e["alert"]

	return isError && !hasID && !isAlert
}

// goldenPasses splits the TS events into passes, each ending with its terminal event.
func goldenPasses(t *testing.T, all []event, passes int) [][]event {
	t.Helper()

	var out [][]event
	begin := 0
	for i, e := range all {
		if terminal(e) {
			out = append(out, all[begin:i+1])
			begin = i + 1
		}
	}
	require.Len(t, all, begin, "golden events after the last pass")
	require.Len(t, out, passes, "golden passes")

	return out
}

// runGoldenPasses runs the scenario's passes on one preview scanner wired as cmd/akt-reporter wires it, and returns
// each pass's events.
func runGoldenPasses(t *testing.T, sc simnode.Scenario) [][]event {
	t.Helper()

	n, err := simnode.Start(t.Context(), sc, simnode.Options{})
	require.NoError(t, err)
	t.Cleanup(n.Close)

	var out bytes.Buffer
	ev := events.NewWriter(&out)
	scanner := newPreviewScanner(t, previewNode{
		cometURL: n.CometURL, grpcAddr: n.GRPCAddr, solanaURL: n.SolanaURL, pin: sc.LightClientPin,
	}, ev)

	for pass := range sc.PassCount() {
		scanner.Pass(t.Context(), nil)
		ev.Emit(json.RawMessage(`{"pass":` + strconv.Itoa(pass+1) + `}`))
	}
	require.NoError(t, ev.Close())

	var passes [][]event
	var current []event
	for _, e := range decodeEvents(t, "Go events", out.Bytes()) {
		if _, ok := e["pass"]; ok {
			passes = append(passes, current)
			current = nil
			continue
		}
		current = append(current, e)
	}
	require.Empty(t, current)

	return passes
}

// previewNode is what a preview scanner is wired against: the node's endpoints and the light client to pin, if any.
type previewNode struct {
	cometURL, grpcAddr, solanaURL string
	pin                           *solana.PublicKey
}

// newPreviewScanner wires a preview scanner writing to ev as cmd/akt-reporter wires it, with the CLI defaults.
func newPreviewScanner(tb testing.TB, n previewNode, ev *events.Writer) *scan.Scanner {
	tb.Helper()

	a, err := akash.Dial(tb.Context(), akash.Options{
		Node:           n.cometURL,
		GRPCAddr:       n.grpcAddr,
		ChainID:        simnode.ChainID,
		MaxAge:         simnode.ReporterMaxAge,
		RequestTimeout: goldenTimeout,
		Concurrency:    goldenAkashReads,
	})
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = a.Close() })

	rpc := aktsolana.NewRPC(aktsolana.RPCOptions{
		URL:              n.solanaURL,
		Timeout:          goldenTimeout,
		Concurrency:      goldenSolanaReads,
		BlockConcurrency: goldenBlockReads,
	})

	var scanner *scan.Scanner
	o := observe.New(rpc, aktsolana.NewEvidenceIndex(rpc, simnode.ProgramID, goldenEvidenceSize, goldenEvidenceMem), observe.Options{
		GenesisHash:    simnode.GenesisHash,
		ProgramID:      simnode.ProgramID,
		Mint:           simnode.MintAddress,
		LightClientPin: n.pin,
		ChainID:        simnode.ChainID,
		MaxAge:         simnode.ReporterMaxAge,
		SlotLead:       goldenSlotLead,
	}, func(c observe.Change) { scanner.OnChange(c) })

	scanner = scan.New(a, o, ev, scan.Options{
		Reporter:     simnode.Reporter,
		Workers:      goldenWorkers,
		PageSize:     goldenPageSize,
		AuditPerScan: goldenAuditPerScan,
	}, nil)

	return scanner
}

// comparePass compares one pass under the ordering rules of spec §9.1: the terminal event is last; events without
// an ID keep their order; each transfer's events keep theirs; transfers listed after an alert in the golden follow it.
func comparePass(t *testing.T, scenario string, pass int, want, got []event) {
	t.Helper()

	where := fmt.Sprintf("%s pass %d", scenario, pass)
	require.NotEmpty(t, got, "%s: no events", where)
	require.True(t, terminal(got[len(got)-1]), "%s: last event %v does not end the pass", where, got[len(got)-1])
	for _, e := range got[:len(got)-1] {
		require.False(t, terminal(e), "%s: %v before the end of the pass", where, e)
	}

	wantPass, wantGroups, wantOrder := split(want)
	gotPass, gotGroups, _ := split(got)

	require.Len(t, gotPass, len(wantPass), "%s: pass events\nwant %v\ngot  %v", where, wantPass, gotPass)
	for i := range wantPass {
		equalEvent(t, where, scenario, "", wantPass[i].event, gotPass[i].event)
	}

	require.ElementsMatch(t, slices.Collect(maps.Keys(wantGroups)), slices.Collect(maps.Keys(gotGroups)),
		"%s: reviewed transfers", where)
	for id, events := range wantGroups {
		require.Len(t, gotGroups[id], len(events), "%s: events of %s", where, id)
		for i := range events {
			equalEvent(t, where, scenario, id, events[i].event, gotGroups[id][i].event)
		}
	}

	// An alert precedes every transfer the golden lists wholly after it.
	for i, alert := range wantPass {
		if terminal(alert.event) {
			continue
		}

		for _, id := range wantOrder {
			if wantGroups[id][0].index < alert.index {
				continue
			}

			for _, e := range gotGroups[id] {
				require.Greater(t, e.index, gotPass[i].index, "%s: %s written before %v", where, id, alert.event)
			}
		}
	}
}

type indexed struct {
	index int
	event event
}

// split returns the events without an ID in order, each transfer's events in order, and the transfers in their first
// event's order.
func split(events []event) (pass []indexed, groups map[string][]indexed, order []string) {
	groups = map[string][]indexed{}
	for i, e := range events {
		id, ok := e["id"].(string)
		if !ok {
			pass = append(pass, indexed{i, e})
			continue
		}

		if _, seen := groups[id]; !seen {
			order = append(order, id)
		}
		groups[id] = append(groups[id], indexed{i, e})
	}

	return pass, groups, order
}

// equalEvent requires got to equal want, field by field, except where an allowance admits a difference.
func equalEvent(t *testing.T, where, scenario, id string, want, got event) {
	t.Helper()

	require.ElementsMatch(t, slices.Collect(maps.Keys(want)), slices.Collect(maps.Keys(got)),
		"%s %s: fields\nwant %v\ngot  %v", where, id, want, got)

	for field, w := range want {
		if text, ok := w.(string); ok && field == "error" {
			w = tsMessage(text)
		}

		if reflect.DeepEqual(w, got[field]) {
			continue
		}
		if a, ok := allowed(scenario, id, field); ok {
			t.Logf("%s %s: field %q differs under §5.6 difference %d", where, id, field, a.difference)
			continue
		}

		require.Failf(t, "event differs from the golden", "%s %s: field %q\nwant %v\ngot  %v\nwhole want %v\nwhole got  %v",
			where, id, field, w, got[field], want, got)
	}
}

// assertDiff matches what node:assert's equal appends after a message, in either of its formats: a "+ actual -
// expected" diff, or a blank line and "<actual> !== <expected>".
var assertDiff = regexp.MustCompile(`\A(\+ actual - expected\n(?s:.*)|\n[^\n]* !== [^\n]*\n)\z`)

// tsMessage strips the node:assert diff from a golden error text. It is the goldens' one normalization that does not
// depend on time: node:assert appends the diff after the TS-authored message, in a format that depends on the Node
// version. It applies to one golden event: wrong-accounts, transfer /11/1 ("mint amount differs from Akash deposit").
// Any other multi-line error text is compared whole.
func tsMessage(text string) string {
	message, rest, ok := strings.Cut(text, "\n")
	if ok && assertDiff.MatchString(rest) {
		return message
	}

	return text
}

func allowed(scenario, id, field string) (allowance, bool) {
	for _, a := range allowances {
		if a.scenario == scenario && a.id == id && a.field == field {
			return a, true
		}
	}

	return allowance{}, false
}
