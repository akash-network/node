package solana_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	evidenceSlot     = 100
	evidenceSlots    = 256
	evidenceBytes    = 64 << 20
	txLegacy         = "legacy"
	instructionError = "InstructionError"
)

type evidenceFixture struct {
	program solana.PublicKey
	address solana.PublicKey
	status  aktsolana.TransferStatus
	node    *fakeNode
	index   *aktsolana.EvidenceIndex
}

func newEvidenceFixture(t *testing.T) *evidenceFixture {
	t.Helper()

	return newBoundedFixture(t, evidenceSlots, evidenceBytes)
}

func newBoundedFixture(t *testing.T, maxSlots int, maxBytes int64) *evidenceFixture {
	t.Helper()

	node, client := newFakeNode(t)
	program := key(0x11)
	owner := bytes.Repeat([]byte{0x02}, 20)

	status := aktsolana.TransferStatus{
		ID:             aktsolana.TransferID{Owner: owner, Height: 100, Sequence: 5},
		ProofHeight:    101,
		ResolvedAtSlot: evidenceSlot,
		Outcome:        aktsolana.Outcome{Minted: &aktsolana.Minted{Amount: 7, Recipient: key(0x09)}},
	}

	return &evidenceFixture{
		program: program,
		address: aktsolana.StatusAddress(program, sinkid.ID{OwnerBytes: owner, Height: 100, Sequence: 5}),
		status:  status,
		node:    node,
		index:   aktsolana.NewEvidenceIndex(client, program, maxSlots, maxBytes),
	}
}

func encodeOutcome(o aktsolana.Outcome) []byte {
	if o.Minted != nil {
		return bytes.Join([][]byte{{outcomeTagMinted}, u64(o.Minted.Amount), o.Minted.Recipient[:]}, nil)
	}

	return []byte{outcomeTagVoided, byte(*o.Voided)}
}

func eventLog(st aktsolana.TransferStatus) string {
	data := bytes.Join([][]byte{
		aktsolana.DiscTransferResolved[:],
		vec(st.ID.Owner), u64(st.ID.Height), u64(st.ID.Sequence),
		u64(st.ProofHeight), u64(st.ResolvedAtSlot),
		encodeOutcome(st.Outcome),
	}, nil)

	return dataPrefix + base64.StdEncoding.EncodeToString(data)
}

func (f *evidenceFixture) logs(st aktsolana.TransferStatus) []string {
	p := f.program.String()

	return []string{fmt.Sprintf("Program %s invoke [1]", p), eventLog(st), fmt.Sprintf("Program %s success", p)}
}

func sigString(fill byte, n uint32) string {
	var s solana.Signature
	for i := range s {
		s[i] = fill
	}
	binary.LittleEndian.PutUint32(s[:4], n)

	return s.String()
}

// resolution is a successful transaction that writes the status account and emits the event for st.
func (f *evidenceFixture) resolution(sig string, st aktsolana.TransferStatus) txSpec {
	return txSpec{
		version: txLegacy, sig: sig, header: [3]int{1, 0, 1},
		static: []solana.PublicKey{key(0x01), f.address, f.program},
		logs:   f.logs(st),
	}
}

func (f *evidenceFixture) lookup(t *testing.T) (solana.Signature, error) {
	t.Helper()

	return f.index.Lookup(context.Background(), evidenceSlot, f.address, f.status)
}

func TestEvidenceFromSolanaTests(t *testing.T) {
	t.Run("observer chooses the successful resolution over a failed replay", func(t *testing.T) {
		f := newEvidenceFixture(t)
		replay := f.resolution(sigString(8, 0), f.status)
		replay.err = map[string]any{instructionError: []any{0, "Custom"}}
		f.node.reply(methodGetBlock, block(replay, f.resolution(sigString(7, 0), f.status)))

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
		require.Equal(t, 1, f.node.count(methodGetBlock))
	})

	t.Run("a version 1 resolution transaction is evidence", func(t *testing.T) {
		f := newEvidenceFixture(t)
		tx := f.resolution(sigString(7, 0), f.status)
		tx.version = 1
		f.node.reply(methodGetBlock, block(tx))

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
	})

	t.Run("spam referencing the status account cannot hide the resolution", func(t *testing.T) {
		f := newEvidenceFixture(t)
		p := f.program.String()

		spam := make([]txSpec, 2000)
		for i := range spam {
			spam[i] = f.resolution(sigString(1, uint32(i)), f.status) //nolint:gosec // small index
			spam[i].logs = []string{fmt.Sprintf("Program %s invoke [1]", p), fmt.Sprintf("Program %s success", p)}
		}

		txs := append(append(append([]txSpec{}, spam...), f.resolution(sigString(7, 0), f.status)), spam...)
		f.node.reply(methodGetBlock, block(txs...))

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
	})

	t.Run("two resolutions for one status fail closed", func(t *testing.T) {
		f := newEvidenceFixture(t)
		f.node.reply(methodGetBlock, block(
			f.resolution(sigString(7, 0), f.status),
			f.resolution(sigString(8, 0), f.status),
		))

		_, err := f.lookup(t)
		require.ErrorIs(t, err, aktsolana.ErrMultipleInBlock)
		require.EqualError(t, err, "multiple resolution transactions in block")
	})

	t.Run("one transaction with two matching events is one resolution", func(t *testing.T) {
		f := newEvidenceFixture(t)
		tx := f.resolution(sigString(7, 0), f.status)
		p := f.program.String()
		tx.logs = []string{
			fmt.Sprintf("Program %s invoke [1]", p), eventLog(f.status), eventLog(f.status),
			fmt.Sprintf("Program %s success", p),
		}
		f.node.reply(methodGetBlock, block(tx))

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
	})

	t.Run("resolution events must match identity, proof height, transaction slot and void reason", func(t *testing.T) {
		governance := aktsolana.VoidGovernance
		invalid := aktsolana.VoidInvalidRecipient

		for name, mutate := range map[string]func(st *aktsolana.TransferStatus){
			"owner":        func(st *aktsolana.TransferStatus) { st.ID.Owner = bytes.Repeat([]byte{9}, 20) },
			"height":       func(st *aktsolana.TransferStatus) { st.ID.Height++ },
			"sequence":     func(st *aktsolana.TransferStatus) { st.ID.Sequence++ },
			"proof":        func(st *aktsolana.TransferStatus) { st.ProofHeight++ },
			"slot differs": func(st *aktsolana.TransferStatus) { st.ResolvedAtSlot++ },
			"amount":       func(st *aktsolana.TransferStatus) { st.Outcome.Minted.Amount++ },
			"recipient": func(st *aktsolana.TransferStatus) {
				st.Outcome.Minted = &aktsolana.Minted{Amount: 7, Recipient: key(0x0a)}
			},
			"void-reason": nil,
			"outcome kind": func(st *aktsolana.TransferStatus) {
				st.Outcome = aktsolana.Outcome{Voided: &governance}
			},
		} {
			t.Run(name, func(t *testing.T) {
				f := newEvidenceFixture(t)
				event := f.status
				event.Outcome = aktsolana.Outcome{Minted: &aktsolana.Minted{Amount: 7, Recipient: key(0x09)}}

				switch {
				case mutate != nil:
					mutate(&event)
				default:
					f.status.Outcome = aktsolana.Outcome{Voided: &governance}
					event.Outcome = aktsolana.Outcome{Voided: &invalid}
				}

				f.node.reply(methodGetBlock, block(f.resolution(sigString(7, 0), event)))

				_, err := f.lookup(t)
				require.ErrorIs(t, err, aktsolana.ErrNotInBlock)
				require.EqualError(t, err, "resolution evidence unavailable in its block")
			})
		}
	})

	t.Run("a voided status matches its voided event", func(t *testing.T) {
		f := newEvidenceFixture(t)
		governance := aktsolana.VoidGovernance
		f.status.Outcome = aktsolana.Outcome{Voided: &governance}
		f.node.reply(methodGetBlock, block(f.resolution(sigString(7, 0), f.status)))

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
	})
}

func TestEvidenceRules(t *testing.T) {
	t.Run("transactions that cannot be evidence", func(t *testing.T) {
		for name, edit := range map[string]func(f *evidenceFixture, tx *txSpec){
			"failed transaction": func(_ *evidenceFixture, tx *txSpec) { tx.err = map[string]any{instructionError: []any{0, "x"}} },
			"no logs":            func(_ *evidenceFixture, tx *txSpec) { tx.logs = nil },
			"read-only":          func(_ *evidenceFixture, tx *txSpec) { tx.header = [3]int{1, 0, 2} },
			"other key":          func(_ *evidenceFixture, tx *txSpec) { tx.static[1] = key(0x77) },
			"event from another program": func(f *evidenceFixture, tx *txSpec) {
				other := key(0x66).String()
				tx.logs = []string{
					fmt.Sprintf("Program %s invoke [1]", other), eventLog(f.status),
					fmt.Sprintf("Program %s success", other),
				}
			},
			"unknown version": func(_ *evidenceFixture, tx *txSpec) { tx.version = 2 },
		} {
			t.Run(name, func(t *testing.T) {
				f := newEvidenceFixture(t)
				tx := f.resolution(sigString(7, 0), f.status)
				edit(f, &tx)
				f.node.reply(methodGetBlock, block(tx))

				_, err := f.lookup(t)
				require.ErrorIs(t, err, aktsolana.ErrNotInBlock)
			})
		}
	})

	t.Run("a transaction with inconsistent logs voids only its own evidence", func(t *testing.T) {
		f := newEvidenceFixture(t)
		p := f.program.String()

		broken := f.resolution(sigString(8, 0), f.status)
		broken.logs = []string{fmt.Sprintf("Program %s invoke [1]", p), eventLog(f.status)}

		malformed := f.resolution(sigString(9, 0), f.status)
		malformed.logs = []string{
			fmt.Sprintf("Program %s invoke [1]", p),
			dataPrefix + base64.StdEncoding.EncodeToString(append(aktsolana.DiscTransferResolved[:], 1, 2, 3)),
			fmt.Sprintf("Program %s success", p),
		}

		f.node.reply(methodGetBlock, block(broken, malformed, f.resolution(sigString(7, 0), f.status)))

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
	})

	t.Run("the status address must be the one the event identifies", func(t *testing.T) {
		f := newEvidenceFixture(t)
		f.node.reply(methodGetBlock, block(f.resolution(sigString(7, 0), f.status)))

		_, err := f.index.Lookup(context.Background(), evidenceSlot, key(0x77), f.status)
		require.ErrorIs(t, err, aktsolana.ErrNotInBlock)
	})

	t.Run("resolution slot must equal the looked-up slot", func(t *testing.T) {
		f := newEvidenceFixture(t)
		f.node.reply(methodGetBlock, block(f.resolution(sigString(7, 0), f.status)))

		_, err := f.index.Lookup(context.Background(), evidenceSlot+1, f.address, f.status)
		require.ErrorContains(t, err, "resolution slot mismatch")
		require.NotErrorIs(t, err, aktsolana.ErrNotInBlock)
	})

	t.Run("the signature must be 64 non-zero bytes", func(t *testing.T) {
		for name, sig := range map[string]string{
			"all zero":        strings.Repeat("1", 64),
			"short":           "sig",
			"not b58":         "0OIl",
			"empty signature": "",
		} {
			t.Run(name, func(t *testing.T) {
				f := newEvidenceFixture(t)
				f.node.reply(methodGetBlock, block(f.resolution(sig, f.status)))

				_, err := f.lookup(t)
				require.Error(t, err)
				require.NotErrorIs(t, err, aktsolana.ErrBlockMissing)
			})
		}
	})
}

func TestResolves(t *testing.T) {
	f := newEvidenceFixture(t)
	events := func(tx aktsolana.Tx) bool {
		return aktsolana.Resolves(tx, f.program, f.address, func(ev aktsolana.TransferResolved) bool {
			return aktsolana.MatchesStatus(ev, f.status)
		})
	}

	other := f.status
	other.ID.Height++

	good := aktsolana.Tx{
		HasLogs:  true,
		Logs:     f.logs(f.status),
		Keys:     []solana.PublicKey{key(1), f.address},
		Writable: []bool{true, true},
	}
	require.True(t, events(good))

	for name, edit := range map[string]func(tx *aktsolana.Tx){
		"failed tx":    func(tx *aktsolana.Tx) { tx.Failed = true },
		"no logs":      func(tx *aktsolana.Tx) { tx.HasLogs = false },
		"read-only":    func(tx *aktsolana.Tx) { tx.Writable = []bool{true, false} },
		"short flags":  func(tx *aktsolana.Tx) { tx.Writable = []bool{true} },
		"missing key":  func(tx *aktsolana.Tx) { tx.Keys = tx.Keys[:1] },
		"broken logs":  func(tx *aktsolana.Tx) { tx.Logs = tx.Logs[:2] },
		"empty":        func(tx *aktsolana.Tx) { tx.Logs = nil },
		"other status": func(tx *aktsolana.Tx) { tx.Logs = f.logs(other) },
	} {
		tx := good
		tx.Keys = append([]solana.PublicKey{}, good.Keys...)
		tx.Writable = append([]bool{}, good.Writable...)
		edit(&tx)
		require.False(t, events(tx), name)
	}

	// A duplicate key that is writable in one position counts, as in web3.js.
	dup := good
	dup.Keys = []solana.PublicKey{f.address, key(1), f.address}
	dup.Writable = []bool{false, true, true}
	require.True(t, events(dup))
}

func TestMatchesStatus(t *testing.T) {
	st := newEvidenceFixture(t).status
	require.True(t, aktsolana.MatchesStatus(aktsolana.TransferResolved(st), st))

	governance := aktsolana.VoidGovernance
	other := aktsolana.VoidInvalidRecipient
	voided := aktsolana.TransferStatus{Outcome: aktsolana.Outcome{Voided: &governance}}
	require.True(t, aktsolana.MatchesStatus(aktsolana.TransferResolved(voided), voided))
	require.False(t, aktsolana.MatchesStatus(
		aktsolana.TransferResolved(aktsolana.TransferStatus{Outcome: aktsolana.Outcome{Voided: &other}}), voided))
	require.False(t, aktsolana.MatchesStatus(aktsolana.TransferResolved(aktsolana.TransferStatus{}), voided))
	require.False(t, aktsolana.MatchesStatus(aktsolana.TransferResolved(aktsolana.TransferStatus{}), aktsolana.TransferStatus{}))
}

func TestEvidenceBlockErrors(t *testing.T) {
	t.Run("a getBlock error is missing history and is not cached", func(t *testing.T) {
		f := newEvidenceFixture(t)
		f.node.reply(methodGetBlock, rpcError())

		_, err := f.lookup(t)
		require.ErrorIs(t, err, aktsolana.ErrBlockMissing)
		require.EqualError(t, err, "resolution evidence unavailable: block missing from owned node history")

		f.node.reply(methodGetBlock, block(f.resolution(sigString(7, 0), f.status)))

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
		require.Equal(t, 2, f.node.count(methodGetBlock))
	})

	t.Run("a stream that fails midway indexes nothing", func(t *testing.T) {
		f := newEvidenceFixture(t)
		whole := block(f.resolution(sigString(7, 0), f.status), f.resolution(sigString(8, 0), f.status))
		f.node.reply(methodGetBlock, whole[:len(whole)-40])

		_, err := f.lookup(t)
		require.ErrorIs(t, err, aktsolana.ErrBlockMissing)

		f.node.reply(methodGetBlock, block(f.resolution(sigString(7, 0), f.status)))

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
		require.Equal(t, 2, f.node.count(methodGetBlock))
	})

	t.Run("a cancelled caller gets its own error and nothing is cached", func(t *testing.T) {
		f := newEvidenceFixture(t)
		f.node.reply(methodGetBlock, block(f.resolution(sigString(7, 0), f.status)))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := f.index.Lookup(ctx, evidenceSlot, f.address, f.status)
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, aktsolana.ErrBlockMissing)

		sig, err := f.lookup(t)
		require.NoError(t, err)
		require.Equal(t, sigString(7, 0), sig.String())
	})
}

func TestIndexEvictsOldestSlot(t *testing.T) {
	f := newBoundedFixture(t, 2, evidenceBytes)
	f.node.reply(methodGetBlock, block())
	lookup := f.lookupSlot(t)

	lookup(1)
	lookup(2)
	require.Equal(t, 2, f.node.count(methodGetBlock))

	lookup(1)
	require.Equal(t, 2, f.node.count(methodGetBlock), "slot 1 is cached")

	lookup(3)
	require.Equal(t, 3, f.node.count(methodGetBlock))

	lookup(2)
	require.Equal(t, 3, f.node.count(methodGetBlock), "slot 2 is cached")

	lookup(1)
	require.Equal(t, 4, f.node.count(methodGetBlock), "slot 1 was evicted although it was used last")
}

func (f *evidenceFixture) lookupSlot(t *testing.T) func(slot uint64) {
	t.Helper()

	return func(slot uint64) {
		st := f.status
		st.ResolvedAtSlot = slot

		_, err := f.index.Lookup(context.Background(), slot, f.address, st)
		require.ErrorIs(t, err, aktsolana.ErrNotInBlock)
	}
}

func TestIndexByteBudget(t *testing.T) {
	// One slot's entries here take about 300 bytes (192 of overhead, the 20 byte owner and the 88 character
	// signature), so 450 holds one slot and not two.
	f := newBoundedFixture(t, evidenceSlots, 450)
	f.node.reply(methodGetBlock, block(f.resolution(sigString(7, 0), f.status)))
	lookup := f.lookupSlot(t)

	lookup(1)
	lookup(2)
	require.Equal(t, 2, f.node.count(methodGetBlock))

	lookup(2)
	require.Equal(t, 2, f.node.count(methodGetBlock), "the higher slot stays")

	lookup(1)
	require.Equal(t, 3, f.node.count(methodGetBlock), "the lower slot was evicted")

	// A slot below every cached one is the lowest, so it is the one that goes.
	lookup(5)
	lookup(3)
	lookup(5)
	require.Equal(t, 5, f.node.count(methodGetBlock))
}

// gatedNode serves one block after its gate opens and counts the getBlock requests it receives.
type gatedNode struct {
	calls   atomic.Int32
	entered chan struct{}
	gate    chan struct{}
	body    string
	client  *aktsolana.RPC
}

func newGatedNode(t *testing.T, body string) *gatedNode {
	t.Helper()

	node := &gatedNode{entered: make(chan struct{}, 64), gate: make(chan struct{}), body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		node.calls.Add(1)
		node.entered <- struct{}{}

		select {
		case <-node.gate:
		case <-r.Context().Done():
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(node.body))
	}))
	t.Cleanup(srv.Close)

	node.client = aktsolana.NewRPC(aktsolana.RPCOptions{
		URL: srv.URL, Timeout: 5 * time.Second, Concurrency: 2, BlockConcurrency: 16,
	})

	return node
}

func TestConcurrentLookupsShareFetch(t *testing.T) {
	f := newEvidenceFixture(t)
	node := newGatedNode(t, block(f.resolution(sigString(7, 0), f.status)))
	index := aktsolana.NewEvidenceIndex(node.client, f.program, evidenceSlots, evidenceBytes)

	const callers = 10

	var (
		wg   sync.WaitGroup
		sigs [callers]string
		errs [callers]error
	)

	for i := range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			sig, err := index.Lookup(context.Background(), evidenceSlot, f.address, f.status)
			sigs[i], errs[i] = sig.String(), err
		}()
	}

	<-node.entered
	time.Sleep(100 * time.Millisecond)
	close(node.gate)
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i])
		require.Equal(t, sigString(7, 0), sigs[i])
	}

	require.EqualValues(t, 1, node.calls.Load())
}

func TestCancelledLeaderDoesNotFailWaiters(t *testing.T) {
	f := newEvidenceFixture(t)
	node := newGatedNode(t, block(f.resolution(sigString(7, 0), f.status)))
	index := aktsolana.NewEvidenceIndex(node.client, f.program, evidenceSlots, evidenceBytes)

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()

	leaderErr := make(chan error, 1)

	go func() {
		_, err := index.Lookup(leaderCtx, evidenceSlot, f.address, f.status)
		leaderErr <- err
	}()

	<-node.entered

	type outcome struct {
		sig solana.Signature
		err error
	}

	waiter := make(chan outcome, 1)

	go func() {
		sig, err := index.Lookup(context.Background(), evidenceSlot, f.address, f.status)
		waiter <- outcome{sig, err}
	}()

	time.Sleep(100 * time.Millisecond)
	cancelLeader()
	require.ErrorIs(t, <-leaderErr, context.Canceled)

	<-node.entered // the waiter fetches for itself
	close(node.gate)

	got := <-waiter
	require.NoError(t, got.err)
	require.Equal(t, sigString(7, 0), got.sig.String())
}
