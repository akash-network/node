package solana_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	methodGetBlock    = "getBlock"
	methodGetTx       = "getTransaction"
	methodGetAccounts = "getMultipleAccounts"
	methodBlockTime   = "getBlockTime"
	methodGenesisHash = "getGenesisHash"
	v1FixtureSig      = "4Ra4SosTpT3Qxc9TzL1MS1YvDwFvuACnGahNK5Hrd9dBSuoRYpBbTDEUqdfN8dU8qKgYJCJQS1rESbBAyz5c81cK"
	testBlockhash     = "11111111111111111111111111111111"
	testSig           = "sig"
	loadedWritable    = "writable"
	loadedReadonly    = "readonly"
)

// fakeNode answers JSON-RPC requests with canned response bodies keyed by method.
type fakeNode struct {
	mu      sync.Mutex
	replies map[string]string
	calls   map[string]int
	params  map[string]json.RawMessage
}

func newFakeNode(t *testing.T) (*fakeNode, *aktsolana.RPC) {
	t.Helper()

	node := &fakeNode{replies: map[string]string{}, calls: map[string]int{}, params: map[string]json.RawMessage{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		node.mu.Lock()
		node.calls[req.Method]++
		node.params[req.Method] = req.Params
		reply, ok := node.replies[req.Method]
		node.mu.Unlock()

		if !ok {
			http.Error(w, "no reply for "+req.Method, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)

	return node, aktsolana.NewRPC(aktsolana.RPCOptions{
		URL:              srv.URL,
		Timeout:          5 * time.Second,
		Concurrency:      2,
		BlockConcurrency: 1,
	})
}

func (n *fakeNode) reply(method, body string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.replies[method] = body
}

func (n *fakeNode) count(method string) int {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.calls[method]
}

func (n *fakeNode) sentParams(method string) json.RawMessage {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.params[method]
}

func result(v any) string {
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": v})
	if err != nil {
		panic(err)
	}

	return string(b)
}

func rpcError() string {
	return `{"jsonrpc":"2.0","id":1,"error":{"code":-32004,"message":"Block not available for slot 7"}}`
}

// txSpec builds the JSON of one transaction as getBlock and getTransaction return it.
type txSpec struct {
	version any
	sig     string
	header  [3]int // numRequiredSignatures, numReadonlySignedAccounts, numReadonlyUnsignedAccounts
	static  []solana.PublicKey
	lookups []map[string]any
	loaded  map[string][]string
	err     any
	noErr   bool // omit meta.err
	logs    []string
}

func (s txSpec) json() map[string]any {
	static := make([]string, len(s.static))
	for i, k := range s.static {
		static[i] = k.String()
	}

	message := map[string]any{
		"accountKeys": static,
		"header": map[string]int{
			"numRequiredSignatures":       s.header[0],
			"numReadonlySignedAccounts":   s.header[1],
			"numReadonlyUnsignedAccounts": s.header[2],
		},
		"instructions":    []any{},
		"recentBlockhash": testBlockhash,
	}
	if s.lookups != nil {
		message["addressTableLookups"] = s.lookups
	}

	meta := map[string]any{"fee": 5000}
	if !s.noErr {
		meta["err"] = s.err
	}
	if s.logs != nil {
		meta["logMessages"] = s.logs
	} else {
		meta["logMessages"] = nil
	}
	if s.loaded != nil {
		meta["loadedAddresses"] = s.loaded
	}

	tx := map[string]any{
		"meta":        meta,
		"transaction": map[string]any{"message": message, "signatures": []string{s.sig}},
	}
	if s.version != nil {
		tx["version"] = s.version
	}

	return tx
}

func (s txSpec) getTransaction(slot uint64) string {
	tx := s.json()
	tx["slot"] = slot
	tx["blockTime"] = 1_700_000_000

	return result(tx)
}

func block(txs ...txSpec) string {
	list := make([]map[string]any, len(txs))
	for i, tx := range txs {
		list[i] = tx.json()
	}

	return result(map[string]any{
		"blockHeight":       9,
		"blockTime":         1_700_000_000,
		"blockhash":         testBlockhash,
		"parentSlot":        9,
		"previousBlockhash": testBlockhash,
		"transactions":      list,
	})
}

func keys(fills ...byte) []solana.PublicKey {
	out := make([]solana.PublicKey, len(fills))
	for i, f := range fills {
		out[i] = key(f)
	}

	return out
}

func collect(t *testing.T, client *aktsolana.RPC, slot uint64) []aktsolana.Tx {
	t.Helper()

	var txs []aktsolana.Tx
	require.NoError(t, client.StreamBlock(context.Background(), slot, func(tx aktsolana.Tx) error {
		txs = append(txs, tx)
		return nil
	}))

	return txs
}

func TestWritability(t *testing.T) {
	ctx := context.Background()

	t.Run("legacy", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetTx, txSpec{
			version: "legacy", sig: testSig, header: [3]int{2, 1, 1}, static: keys(1, 2, 3, 4),
			loaded: map[string][]string{loadedWritable: {}, loadedReadonly: {}}, logs: []string{},
		}.getTransaction(11))

		tx, err := client.Transaction(ctx, testSig)
		require.NoError(t, err)
		require.NotNil(t, tx)
		require.Equal(t, "legacy", tx.Version)
		require.Equal(t, uint64(11), tx.Slot)
		require.Equal(t, keys(1, 2, 3, 4), tx.Keys)
		require.Equal(t, []bool{true, false, true, false}, tx.Writable)
	})

	t.Run("v0 with loaded addresses", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetTx, txSpec{
			version: 0, sig: testSig, header: [3]int{1, 0, 1}, static: keys(1, 2, 3),
			lookups: []map[string]any{{
				"accountKey": key(9).String(), "writableIndexes": []int{0}, "readonlyIndexes": []int{1, 2},
			}},
			loaded: map[string][]string{loadedWritable: {key(4).String()}, loadedReadonly: {key(5).String(), key(6).String()}},
			logs:   []string{},
		}.getTransaction(12))

		tx, err := client.Transaction(ctx, testSig)
		require.NoError(t, err)
		require.Equal(t, "0", tx.Version)
		require.Equal(t, keys(1, 2, 3, 4, 5, 6), tx.Keys)
		require.Equal(t, []bool{true, true, false, true, false, false}, tx.Writable)
	})

	t.Run("v0 with unresolved lookups", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetTx, txSpec{
			version: 0, sig: testSig, header: [3]int{1, 0, 0}, static: keys(1, 2),
			lookups: []map[string]any{{
				"accountKey": key(9).String(), "writableIndexes": []int{0, 1}, "readonlyIndexes": []int{},
			}},
			loaded: map[string][]string{loadedWritable: {key(4).String()}, loadedReadonly: {}},
			logs:   []string{},
		}.getTransaction(12))

		tx, err := client.Transaction(ctx, testSig)
		require.NoError(t, err)
		require.Len(t, tx.Writable, len(tx.Keys))
		require.NotContains(t, tx.Writable, true)
	})

	t.Run("a version 1 resolution transaction is evidence", func(t *testing.T) {
		fixture, err := os.ReadFile("testdata/getblock_v1.json")
		require.NoError(t, err)

		node, client := newFakeNode(t)
		node.reply(methodGetBlock, string(fixture))

		txs := collect(t, client, 147)
		require.Len(t, txs, 2)

		vote := txs[0]
		require.Equal(t, "legacy", vote.Version)
		require.Equal(t, []bool{true, true, false}, vote.Writable)

		transfer := txs[1]
		require.Equal(t, "1", transfer.Version)
		require.Equal(t, uint64(147), transfer.Slot)
		require.Equal(t, []string{v1FixtureSig}, transfer.Signatures)
		require.False(t, transfer.Failed)
		require.True(t, transfer.HasLogs)
		require.Equal(t, []solana.PublicKey{
			solana.MustPublicKeyFromBase58("E99hNwo5WKJQ1u4EayxBJcWiM8Z8CS3rmxW9Nk8b9h9M"),
			solana.MustPublicKeyFromBase58("BYMt4qh6Si2NzZbJUWv8Q5dJjnGPKG1CFSHPC7j5p2om"),
			solana.SystemProgramID,
		}, transfer.Keys)
		require.Equal(t, []bool{true, true, false}, transfer.Writable)
	})
}

func TestStreamBlockOrderAndFields(t *testing.T) {
	node, client := newFakeNode(t)
	node.reply(methodGetBlock, block(
		txSpec{
			version: "legacy", sig: "failed", header: [3]int{1, 0, 0}, static: keys(1),
			err: map[string]any{"InstructionError": []any{0, "Custom"}}, logs: []string{"failed log"},
		},
		txSpec{version: 0, sig: "nologs", header: [3]int{1, 0, 0}, static: keys(2)},
		txSpec{version: 1, sig: "success", header: [3]int{1, 0, 0}, static: keys(3), logs: []string{"a", "b"}},
		txSpec{version: 1, sig: "no err", header: [3]int{1, 0, 0}, static: keys(4), noErr: true, logs: []string{}},
	))

	txs := collect(t, client, 10)
	require.Len(t, txs, 4)
	require.True(t, txs[3].Failed, "an absent meta.err is not success")
	txs = txs[:3]

	require.Equal(t, []string{"failed"}, txs[0].Signatures)
	require.True(t, txs[0].Failed)
	require.True(t, txs[0].HasLogs)
	require.Equal(t, []string{"failed log"}, txs[0].Logs)

	require.Equal(t, []string{"nologs"}, txs[1].Signatures)
	require.False(t, txs[1].Failed)
	require.False(t, txs[1].HasLogs)
	require.Empty(t, txs[1].Logs)

	require.Equal(t, []string{"success"}, txs[2].Signatures)
	require.False(t, txs[2].Failed)
	require.True(t, txs[2].HasLogs)
	require.Equal(t, []string{"a", "b"}, txs[2].Logs)

	for _, tx := range txs {
		require.Equal(t, uint64(10), tx.Slot)
	}

	require.JSONEq(t, `[10, {
		"commitment": "finalized",
		"encoding": "json",
		"maxSupportedTransactionVersion": 1,
		"transactionDetails": "full",
		"rewards": false
	}]`, string(node.sentParams(methodGetBlock)))

	stop := errors.New("stop")
	calls := 0
	err := client.StreamBlock(context.Background(), 10, func(aktsolana.Tx) error {
		calls++
		return stop
	})
	require.ErrorIs(t, err, stop)
	require.Equal(t, 1, calls)
}

func TestUnknownVersionSuppliesNoEvidence(t *testing.T) {
	node, client := newFakeNode(t)
	node.reply(methodGetBlock, block(
		txSpec{version: 2, sig: "future", header: [3]int{2, 0, 0}, static: keys(1, 2, 3), logs: []string{}},
		txSpec{sig: "unversioned", header: [3]int{1, 0, 0}, static: keys(4), logs: []string{}},
	))

	txs := collect(t, client, 10)
	require.Len(t, txs, 2)
	require.Equal(t, "2", txs[0].Version)
	require.Equal(t, []bool{false, false, false}, txs[0].Writable)
	require.Equal(t, []bool{false}, txs[1].Writable)
}

func TestErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("an RPC error is unavailability", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetAccounts, rpcError())

		_, _, err := client.MultipleAccounts(ctx, keys(1))
		require.ErrorIs(t, err, aktsolana.ErrUnavailable)
		require.EqualError(t, err, "Solana node is temporarily unavailable")
	})

	t.Run("an HTTP error is unavailability", func(t *testing.T) {
		_, client := newFakeNode(t)

		_, err := client.GenesisHash(ctx)
		require.ErrorIs(t, err, aktsolana.ErrUnavailable)
	})

	t.Run("a getBlock error is a missing block", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetBlock, rpcError())

		err := client.StreamBlock(ctx, 7, func(aktsolana.Tx) error { return nil })
		require.ErrorIs(t, err, aktsolana.ErrBlockUnavailable)
	})

	t.Run("a null block is a missing block", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetBlock, result(nil))

		err := client.StreamBlock(ctx, 7, func(aktsolana.Tx) error { return nil })
		require.ErrorIs(t, err, aktsolana.ErrBlockUnavailable)
	})

	t.Run("a truncated block is a missing block", func(t *testing.T) {
		node, client := newFakeNode(t)
		full := block(txSpec{version: 0, sig: "a", header: [3]int{1, 0, 0}, static: keys(1)})
		node.reply(methodGetBlock, full[:len(full)-10])

		err := client.StreamBlock(ctx, 7, func(aktsolana.Tx) error { return nil })
		require.ErrorIs(t, err, aktsolana.ErrBlockUnavailable)
	})

	t.Run("more than 100 keys", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetAccounts, result(nil))

		_, _, err := client.MultipleAccounts(ctx, make([]solana.PublicKey, 101))
		require.Error(t, err)
		require.NotErrorIs(t, err, aktsolana.ErrUnavailable)
		require.Zero(t, node.count(methodGetAccounts))
	})
}

func TestBlockTimeCached(t *testing.T) {
	ctx := context.Background()
	node, client := newFakeNode(t)
	node.reply(methodBlockTime, result(1_700_000_000))

	for range 2 {
		got, err := client.BlockTime(ctx, 7)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, int64(1_700_000_000), *got)
	}
	require.Equal(t, 1, node.count(methodBlockTime))
	require.JSONEq(t, `[7]`, string(node.sentParams(methodBlockTime)))

	node.reply(methodBlockTime, result(nil))
	got, err := client.BlockTime(ctx, 8)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestReads(t *testing.T) {
	ctx := context.Background()

	t.Run("genesis hash", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGenesisHash, result("5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"))

		hash, err := client.GenesisHash(ctx)
		require.NoError(t, err)
		require.Equal(t, "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d", hash)
	})

	t.Run("multiple accounts", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetAccounts, result(map[string]any{
			"context": map[string]any{"slot": 42},
			"value": []any{
				map[string]any{
					"data": []string{"AQID", "base64"}, "executable": true, "lamports": 1,
					"owner": key(7).String(), "rentEpoch": 0, "space": 3,
				},
				nil,
			},
		}))

		slot, accts, err := client.MultipleAccounts(ctx, keys(1, 2))
		require.NoError(t, err)
		require.Equal(t, uint64(42), slot)
		require.Equal(t, []*aktsolana.Account{{Owner: key(7), Executable: true, Data: []byte{1, 2, 3}}, nil}, accts)
		require.JSONEq(t, `[["`+key(1).String()+`","`+key(2).String()+`"],{"commitment":"finalized","encoding":"base64"}]`,
			string(node.sentParams(methodGetAccounts)))
	})

	t.Run("an account count mismatch is unavailability", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetAccounts, result(map[string]any{"context": map[string]any{"slot": 42}, "value": []any{nil}}))

		_, _, err := client.MultipleAccounts(ctx, keys(1, 2))
		require.ErrorIs(t, err, aktsolana.ErrUnavailable)
	})

	t.Run("a missing transaction", func(t *testing.T) {
		node, client := newFakeNode(t)
		node.reply(methodGetTx, result(nil))

		tx, err := client.Transaction(ctx, testSig)
		require.NoError(t, err)
		require.Nil(t, tx)
		require.JSONEq(t, `["sig",{"commitment":"finalized","encoding":"json","maxSupportedTransactionVersion":1}]`,
			string(node.sentParams(methodGetTx)))
	})
}
