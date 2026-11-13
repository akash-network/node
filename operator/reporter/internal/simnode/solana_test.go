package simnode_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	genesisHash  = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"
	finalSlot    = 120
	resolveSlot  = 110
	legacySig    = "5pHmPtZoSanisJdZ2StPm4TKszc2hRMHjhq9GznkwbxzMFbKGT3hNa3eKrYVkDJTQT2LF4RxC7zJkZCMANsgZHLA"
	v0Sig        = "4PTotq7RjmePqkh5to6jCmyiN6khRshVyBAusJScaUyJUNSBYbCctgEnP7Pa5kpyG6NxMhpBhJK7JSaoAPjs2P7J"
	v1Sig        = "4Ra4SosTpT3Qxc9TzL1MS1YvDwFvuACnGahNK5Hrd9dBSuoRYpBbTDEUqdfN8dU8qKgYJCJQS1rESbBAyz5c81cK"
	legacyOnly   = 115
	legacyOnlySg = "2xGCrQqp7yS9GB9XyPV3Mxft3UKP2oe2DCYxCFV4ZN1rW2c3w2Hw5ntv6hmhPqZRXu7AD2dYq5bQX9ynZphRQ6Ln"

	cfgCommitment = "commitment"
	cfgEncoding   = "encoding"
	cfgMaxVersion = "maxSupportedTransactionVersion"
	finalized     = "finalized"
	encJSON       = "json"
)

var start = time.Date(2026, 11, 12, 10, 0, 0, 700_000_000, time.UTC)

type fixture struct {
	program, config, bridge, mint, status, recipient, payer, light, lookup solana.PublicKey
	id                                                                     sinkid.ID
	launch                                                                 simnode.LaunchConfig
	bridgeCfg                                                              simnode.BridgeConfig
	mintAcct                                                               simnode.Mint
	resolved                                                               simnode.TransferStatus
}

func key(b byte) solana.PublicKey {
	var k solana.PublicKey
	for i := range k {
		k[i] = b
	}

	return k
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	owner := bytes.Repeat([]byte{7}, 20)
	addr, err := bech32.ConvertAndEncode("akash", owner)
	require.NoError(t, err)

	f := fixture{
		program:   key(1),
		recipient: key(2),
		payer:     key(3),
		light:     key(4),
		lookup:    key(5),
		id:        sinkid.ID{Owner: addr, OwnerBytes: owner, Height: 42, Sequence: 7},
	}
	f.config = aktsolana.PDA(f.program, []byte("config"))
	f.bridge = aktsolana.PDA(f.program, []byte("bridge"))
	f.mint = aktsolana.PDA(f.program, []byte("mint"))
	f.status = aktsolana.StatusAddress(f.program, f.id)
	authority := aktsolana.PDA(f.program, []byte("mint_authority"))

	f.launch = simnode.LaunchConfig{
		Version: 2, Initializer: f.payer, Mint: f.mint, BridgeEnabled: true,
		InitializedAtSlot: 3, Bump: 254, MintAuthorityBump: 253,
	}
	f.bridgeCfg = simnode.BridgeConfig{
		LightClientProgram: f.light, ChainID: "akashnet-2",
		PerTransferMax: 1_000_000, DailyMintCap: 5_000_000, Vaults: []solana.PublicKey{key(9)},
		MintDay: 20_000, MintedToday: 12,
	}
	f.mintAcct = simnode.Mint{MintAuthority: &authority, Supply: 1 << 40, Decimals: 6, Initialized: true}
	f.resolved = simnode.TransferStatus{
		ID: f.id, ProofHeight: 43, ResolvedAtSlot: resolveSlot,
		Outcome: simnode.Outcome{Minted: &simnode.Minted{Amount: 1_000_000, Recipient: f.recipient}},
	}

	return f
}

func (f fixture) logs() []string {
	p := f.program.String()

	return []string{
		"Program " + p + " invoke [1]",
		"Program log: Instruction: MintTransfer",
		simnode.TransferResolvedLog(f.resolved),
		"Program " + p + " success",
	}
}

func (f fixture) scenario() simnode.Scenario {
	keys := []solana.PublicKey{f.payer, f.status, f.program}
	header := simnode.Header{NumRequiredSignatures: 1, NumReadonlyUnsignedAccounts: 1}

	return simnode.Scenario{
		Solana: simnode.Solana{
			GenesisHash: genesisHash,
			Slot:        finalSlot,
			BlockTimes: map[uint64]time.Duration{
				resolveSlot: -30 * time.Second,
				legacyOnly:  -20 * time.Second,
				finalSlot:   -5 * time.Second,
			},
			Accounts: map[solana.PublicKey]simnode.Account{
				f.program: {Owner: solana.BPFLoaderUpgradeableProgramID, Executable: true, Lamports: 1, Data: []byte{2, 0, 0, 0}},
				f.config:  {Owner: f.program, Lamports: 2, Data: simnode.EncodeLaunchConfig(f.launch)},
				f.bridge:  {Owner: f.program, Lamports: 3, Data: simnode.EncodeBridgeConfig(f.bridgeCfg)},
				f.mint:    {Owner: solana.TokenProgramID, Lamports: 4, Data: simnode.EncodeMint(f.mintAcct)},
				f.status:  {Owner: f.program, Lamports: 5, Data: simnode.EncodeTransferStatus(f.resolved)},
			},
			Blocks: map[uint64]simnode.Block{
				resolveSlot: {Transactions: []simnode.Transaction{
					{
						Signatures: []string{legacySig}, Version: "legacy", Header: header, AccountKeys: keys,
						Err: json.RawMessage(`{"InstructionError":[0,{"Custom":6001}]}`), Logs: f.logs(),
					},
					{
						Signatures: []string{v0Sig}, Version: "0",
						Header:      simnode.Header{NumRequiredSignatures: 1},
						AccountKeys: []solana.PublicKey{f.payer, f.program},
						AddressTableLookups: []simnode.AddressTableLookup{
							{AccountKey: f.lookup, WritableIndexes: []int{0}, ReadonlyIndexes: []int{1}},
						},
						LoadedWritable: []solana.PublicKey{f.status},
						LoadedReadonly: []solana.PublicKey{f.recipient},
						Logs:           f.logs(),
					},
					{Signatures: []string{v1Sig}, Version: "1", Header: header, AccountKeys: keys, Logs: f.logs()},
				}},
				legacyOnly: {Transactions: []simnode.Transaction{
					{Signatures: []string{legacyOnlySg}, Header: header, AccountKeys: keys},
				}},
			},
		},
	}
}

func startNode(t *testing.T, sc simnode.Scenario, opts simnode.Options) *simnode.Node {
	t.Helper()

	n, err := simnode.Start(t.Context(), sc, opts)
	require.NoError(t, err)
	t.Cleanup(n.Close)

	return n
}

func client(n *simnode.Node) *aktsolana.RPC {
	return aktsolana.NewRPC(aktsolana.RPCOptions{URL: n.SolanaURL, Timeout: 5 * time.Second, Concurrency: 4, BlockConcurrency: 2})
}

type rpcReply struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func call(t *testing.T, url, method string, params ...any) rpcReply {
	t.Helper()

	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out rpcReply
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))

	return out
}

var blockConfig = map[string]any{
	cfgCommitment: finalized, cfgEncoding: encJSON, "transactionDetails": "full",
	"rewards": false, cfgMaxVersion: 1,
}

func TestSolanaMethods(t *testing.T) {
	f := newFixture(t)
	n := startNode(t, f.scenario(), simnode.Options{Clock: clock.NewFake(start)})
	rpc := client(n)
	ctx := t.Context()

	require.Empty(t, n.CometURL)
	require.Empty(t, n.GRPCAddr)

	t.Run("getGenesisHash", func(t *testing.T) {
		hash, err := rpc.GenesisHash(ctx)
		require.NoError(t, err)
		require.Equal(t, genesisHash, hash)
	})

	t.Run("getMultipleAccounts round-trips through the production decoders", func(t *testing.T) {
		missing := key(0xee)
		slot, accts, err := rpc.MultipleAccounts(ctx, []solana.PublicKey{f.program, f.config, f.bridge, f.mint, f.status, missing})
		require.NoError(t, err)
		require.EqualValues(t, finalSlot, slot)
		require.Len(t, accts, 6)
		require.Nil(t, accts[5])

		require.True(t, accts[0].Executable)
		require.Equal(t, solana.BPFLoaderUpgradeableProgramID, accts[0].Owner)

		require.Len(t, accts[1].Data, 84)
		launch, err := aktsolana.DecodeLaunchConfig(accts[1].Data)
		require.NoError(t, err)
		require.Equal(t, aktsolana.LaunchConfig{
			Version: 2, Initializer: f.payer, Mint: f.mint, BridgeEnabled: true,
			InitializedAtSlot: 3, Bump: 254, MintAuthorityBump: 253,
		}, launch)
		require.Equal(t, f.program, accts[1].Owner)

		require.Len(t, accts[2].Data, 656)
		bridge, err := aktsolana.DecodeBridgeConfig(accts[2].Data)
		require.NoError(t, err)
		require.Equal(t, aktsolana.BridgeConfig{LightClientProgram: f.light, ChainID: "akashnet-2"}, bridge)

		mint, err := aktsolana.DecodeMint(accts[3].Data)
		require.NoError(t, err)
		require.Equal(t, aktsolana.Mint{MintAuthority: f.mintAcct.MintAuthority, Supply: 1 << 40, Decimals: 6, Initialized: true}, mint)
		require.Equal(t, solana.TokenProgramID, accts[3].Owner)

		require.Len(t, accts[4].Data, 340)
		status, err := aktsolana.DecodeTransferStatus(accts[4].Data)
		require.NoError(t, err)
		require.Equal(t, aktsolana.TransferStatus{
			ID:          aktsolana.TransferID{Owner: f.id.OwnerBytes, Height: 42, Sequence: 7},
			ProofHeight: 43, ResolvedAtSlot: resolveSlot,
			Outcome: aktsolana.Outcome{Minted: &aktsolana.Minted{Amount: 1_000_000, Recipient: f.recipient}},
		}, status)
	})

	t.Run("voided outcomes round-trip", func(t *testing.T) {
		for _, reason := range []simnode.VoidReason{simnode.VoidInvalidRecipient, simnode.VoidGovernance} {
			st := f.resolved
			st.Outcome = simnode.Outcome{Voided: &reason}

			decoded, err := aktsolana.DecodeTransferStatus(simnode.EncodeTransferStatus(st))
			require.NoError(t, err)
			require.NotNil(t, decoded.Outcome.Voided)
			require.EqualValues(t, reason, *decoded.Outcome.Voided)

			event, err := aktsolana.DecodeTransferResolved(simnode.EncodeTransferResolved(st))
			require.NoError(t, err)
			require.Equal(t, aktsolana.TransferResolved(decoded), event)
		}
	})

	t.Run("getBlockTime is null for unknown slots", func(t *testing.T) {
		bt, err := rpc.BlockTime(ctx, finalSlot)
		require.NoError(t, err)
		require.NotNil(t, bt)
		require.Equal(t, start.Add(-5*time.Second).Unix(), *bt)

		bt, err = rpc.BlockTime(ctx, 999)
		require.NoError(t, err)
		require.Nil(t, bt)
	})

	t.Run("getBlock streams every version with its writability", func(t *testing.T) {
		var txs []aktsolana.Tx
		require.NoError(t, rpc.StreamBlock(ctx, resolveSlot, func(tx aktsolana.Tx) error {
			txs = append(txs, tx)
			return nil
		}))
		require.Len(t, txs, 3)

		require.Equal(t, "legacy", txs[0].Version)
		require.True(t, txs[0].Failed)
		require.Equal(t, []bool{true, true, false}, txs[0].Writable)

		require.Equal(t, "0", txs[1].Version)
		require.False(t, txs[1].Failed)
		require.Equal(t, []solana.PublicKey{f.payer, f.program, f.status, f.recipient}, txs[1].Keys)
		require.Equal(t, []bool{true, true, true, false}, txs[1].Writable)

		require.Equal(t, "1", txs[2].Version)
		require.Equal(t, []string{v1Sig}, txs[2].Signatures)
		require.True(t, txs[2].HasLogs)

		events, err := aktsolana.ProgramEvents(txs[2].Logs, f.program)
		require.NoError(t, err)
		require.Len(t, events, 1)
		st, err := aktsolana.DecodeTransferStatus(simnode.EncodeTransferStatus(f.resolved))
		require.NoError(t, err)
		require.True(t, aktsolana.MatchesStatus(events[0], st))
		require.True(t, aktsolana.Resolves(txs[2], f.program, f.status, func(ev aktsolana.TransferResolved) bool {
			return aktsolana.MatchesStatus(ev, st)
		}))
	})

	t.Run("getBlock of a missing or unfinalized slot fails", func(t *testing.T) {
		err := rpc.StreamBlock(ctx, 111, func(aktsolana.Tx) error { return nil })
		require.ErrorIs(t, err, aktsolana.ErrBlockUnavailable)

		reply := call(t, n.SolanaURL, "getBlock", 111, blockConfig)
		require.NotNil(t, reply.Error)
		require.Equal(t, -32007, reply.Error.Code)
		require.Equal(t, "Slot 111 was skipped, or missing due to ledger jump to recent snapshot", reply.Error.Message)

		reply = call(t, n.SolanaURL, "getBlock", finalSlot+1, blockConfig)
		require.NotNil(t, reply.Error)
		require.Equal(t, -32004, reply.Error.Code)
		require.Equal(t, "Block not available for slot 121", reply.Error.Message)
	})

	t.Run("getTransaction", func(t *testing.T) {
		tx, err := rpc.Transaction(ctx, v0Sig)
		require.NoError(t, err)
		require.NotNil(t, tx)
		require.EqualValues(t, resolveSlot, tx.Slot)
		require.Equal(t, "0", tx.Version)
		require.Equal(t, []bool{true, true, true, false}, tx.Writable)

		tx, err = rpc.Transaction(ctx, v1Sig)
		require.NoError(t, err)
		require.Equal(t, "1", tx.Version)

		tx, err = rpc.Transaction(ctx, "3Bxs4Bc3VYuGVB19")
		require.NoError(t, err)
		require.Nil(t, tx)

		reply := call(t, n.SolanaURL, "getTransaction", legacySig, map[string]any{cfgEncoding: encJSON, cfgMaxVersion: 1})
		require.Nil(t, reply.Error)
		var res struct {
			BlockTime int64 `json:"blockTime"`
			Slot      uint64
		}
		require.NoError(t, json.Unmarshal(reply.Result, &res))
		require.Equal(t, start.Add(-30*time.Second).Unix(), res.BlockTime)
	})

	t.Run("the version is honored", func(t *testing.T) {
		noVersion := map[string]any{cfgCommitment: finalized, cfgEncoding: encJSON, "rewards": false}

		reply := call(t, n.SolanaURL, "getBlock", resolveSlot, noVersion)
		require.NotNil(t, reply.Error)
		require.Equal(t, -32015, reply.Error.Code)
		require.Equal(t, `Transaction version (0) is not supported by the requesting client. `+
			`Please try the request again with the following configuration parameter: "maxSupportedTransactionVersion": 0`,
			reply.Error.Message)

		reply = call(t, n.SolanaURL, "getBlock", resolveSlot, map[string]any{cfgEncoding: encJSON, cfgMaxVersion: 0})
		require.NotNil(t, reply.Error)
		require.Equal(t, -32015, reply.Error.Code)
		require.Contains(t, reply.Error.Message, "Transaction version (1)")

		reply = call(t, n.SolanaURL, "getTransaction", v1Sig, map[string]any{cfgEncoding: encJSON})
		require.NotNil(t, reply.Error)
		require.Equal(t, -32015, reply.Error.Code)

		// A legacy-only block answers a client without the parameter, and omits the version field.
		reply = call(t, n.SolanaURL, "getBlock", legacyOnly, noVersion)
		require.Nil(t, reply.Error)
		require.NotContains(t, string(reply.Result), `"version"`)

		reply = call(t, n.SolanaURL, "getBlock", legacyOnly, blockConfig)
		require.Nil(t, reply.Error)
		require.Contains(t, string(reply.Result), `"version":"legacy"`)

		reply = call(t, n.SolanaURL, "getBlock", legacyOnly, map[string]any{cfgEncoding: "base64", cfgMaxVersion: 1})
		require.NotNil(t, reply.Error)
		require.Equal(t, -32602, reply.Error.Code)
	})

	t.Run("the JSON shapes match the Agave capture", func(t *testing.T) {
		raw, err := os.ReadFile("../../solana/testdata/getblock_v1.json")
		require.NoError(t, err)
		var want struct {
			Result map[string]json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal(raw, &want))

		reply := call(t, n.SolanaURL, "getBlock", resolveSlot, blockConfig)
		require.Nil(t, reply.Error)
		var got map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(reply.Result, &got))

		require.Equal(t, keysOf(t, want.Result), keysOf(t, got))

		var wantTxs, gotTxs []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(want.Result["transactions"], &wantTxs))
		require.NoError(t, json.Unmarshal(got["transactions"], &gotTxs))
		// The capture holds a legacy and a v1 transaction; ours a legacy, a v0 and a v1 one.
		for _, pair := range [][2]map[string]json.RawMessage{{wantTxs[0], gotTxs[0]}, {wantTxs[1], gotTxs[2]}} {
			require.Equal(t, keysOf(t, pair[0]), keysOf(t, pair[1]))
			require.Equal(t, keysOf(t, sub(t, pair[0], "meta")), keysOf(t, sub(t, pair[1], "meta")))
			wantTx, gotTx := sub(t, pair[0], "transaction"), sub(t, pair[1], "transaction")
			require.Equal(t, keysOf(t, wantTx), keysOf(t, gotTx))
			require.Equal(t, keysOf(t, sub(t, wantTx, "message")), keysOf(t, sub(t, gotTx, "message")))
		}
		require.Equal(t, "1", string(gotTxs[2]["version"]))
		require.JSONEq(t, `{"Ok":null}`, string(sub(t, gotTxs[2], "meta")["status"]))
	})

	t.Run("calls are counted per method", func(t *testing.T) {
		require.Positive(t, n.Calls("getGenesisHash"))
		require.Positive(t, n.Calls("getBlock"))
		require.Zero(t, n.Calls("getSlot"))
	})
}

func keysOf(t *testing.T, m map[string]json.RawMessage) []string {
	t.Helper()

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}

func sub(t *testing.T, m map[string]json.RawMessage, field string) map[string]json.RawMessage {
	t.Helper()

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(m[field], &out), field)

	return out
}

func TestGetMultipleAccountsLimit(t *testing.T) {
	f := newFixture(t)
	n := startNode(t, f.scenario(), simnode.Options{})

	keys := make([]string, 101)
	for i := range keys {
		keys[i] = f.status.String()
	}
	cfg := map[string]any{cfgCommitment: finalized, cfgEncoding: "base64"}

	reply := call(t, n.SolanaURL, "getMultipleAccounts", keys, cfg)
	require.NotNil(t, reply.Error)
	require.Equal(t, -32602, reply.Error.Code)
	require.Equal(t, "Too many inputs provided; max 100", reply.Error.Message)

	reply = call(t, n.SolanaURL, "getMultipleAccounts", keys[:100], cfg)
	require.Nil(t, reply.Error)
	require.Equal(t, 2, n.Calls("getMultipleAccounts"))
}

func TestLatencyInjection(t *testing.T) {
	f := newFixture(t)
	n := startNode(t, f.scenario(), simnode.Options{SolanaLatency: 5 * time.Millisecond, BlockFetchLatency: 20 * time.Millisecond})
	rpc := client(n)

	for range 3 {
		began := time.Now()
		_, err := rpc.GenesisHash(t.Context())
		require.NoError(t, err)
		require.GreaterOrEqual(t, time.Since(began), 5*time.Millisecond)

		began = time.Now()
		_, _, err = rpc.MultipleAccounts(t.Context(), []solana.PublicKey{f.status})
		require.NoError(t, err)
		require.GreaterOrEqual(t, time.Since(began), 5*time.Millisecond)
	}

	began := time.Now()
	require.NoError(t, rpc.StreamBlock(t.Context(), legacyOnly, func(aktsolana.Tx) error { return nil }))
	require.GreaterOrEqual(t, time.Since(began), 25*time.Millisecond)
}

func TestTimesRebased(t *testing.T) {
	f := newFixture(t)
	fake := clock.NewFake(start)
	n := startNode(t, f.scenario(), simnode.Options{Clock: fake})
	rpc := client(n)

	want := start.Add(-5 * time.Second).Unix()
	bt, err := rpc.BlockTime(t.Context(), finalSlot)
	require.NoError(t, err)
	require.Equal(t, want, *bt)

	// Whole seconds, rounded down as Agave's Unix timestamps are.
	require.Equal(t, int64(1794477595), want)

	// The base is fixed at Start: a later clock does not move recorded block times.
	fake.Advance(time.Hour)
	reply := call(t, n.SolanaURL, "getBlockTime", finalSlot)
	require.Nil(t, reply.Error)
	require.Equal(t, "1794477595", string(reply.Result))

	reply = call(t, n.SolanaURL, "getBlock", legacyOnly, blockConfig)
	require.Nil(t, reply.Error)
	var block struct {
		BlockTime int64 `json:"blockTime"`
	}
	require.NoError(t, json.Unmarshal(reply.Result, &block))
	require.Equal(t, start.Add(-20*time.Second).Unix(), block.BlockTime)
}

func TestFailureInjectionAndMutate(t *testing.T) {
	f := newFixture(t)
	sc := f.scenario()
	sc.Solana.Failures = map[string]simnode.Failure{
		"getGenesisHash":      {Code: -32005, Message: "Node is unhealthy", Times: 2},
		"getMultipleAccounts": {HTTPStatus: http.StatusServiceUnavailable},
	}
	n := startNode(t, sc, simnode.Options{})
	rpc := client(n)
	ctx := t.Context()

	for range 2 {
		_, err := rpc.GenesisHash(ctx)
		require.ErrorIs(t, err, aktsolana.ErrUnavailable)
		require.Contains(t, chainText(err), "RPC error -32005: Node is unhealthy")
	}
	hash, err := rpc.GenesisHash(ctx)
	require.NoError(t, err)
	require.Equal(t, genesisHash, hash)
	require.Equal(t, 3, n.Calls("getGenesisHash"))

	_, _, err = rpc.MultipleAccounts(ctx, []solana.PublicKey{f.status})
	require.ErrorIs(t, err, aktsolana.ErrUnavailable)

	n.Mutate(func(s *simnode.Scenario) {
		delete(s.Solana.Failures, "getMultipleAccounts")
		s.Solana.Slot = 130
		delete(s.Solana.Accounts, f.status)
		b := s.Solana.Blocks[legacyOnly]
		b.Transactions = append(b.Transactions, simnode.Transaction{
			Signatures: []string{v0Sig + "x"}, Header: simnode.Header{NumRequiredSignatures: 1},
			AccountKeys: []solana.PublicKey{f.payer},
		})
		s.Solana.Blocks[legacyOnly] = b
		delete(s.Solana.Blocks, resolveSlot)
	})

	slot, accts, err := rpc.MultipleAccounts(ctx, []solana.PublicKey{f.status})
	require.NoError(t, err)
	require.EqualValues(t, 130, slot)
	require.Nil(t, accts[0])

	tx, err := rpc.Transaction(ctx, v0Sig)
	require.NoError(t, err)
	require.Nil(t, tx)
	tx, err = rpc.Transaction(ctx, v0Sig+"x")
	require.NoError(t, err)
	require.EqualValues(t, legacyOnly, tx.Slot)
}

func TestConcurrentRequestsMutateAndFailures(t *testing.T) {
	const (
		workers   = 8
		rounds    = 20
		failTimes = 10
	)

	f := newFixture(t)
	sc := f.scenario()
	sc.Solana.Failures = map[string]simnode.Failure{"getGenesisHash": {Code: -32005, Message: "Node is unhealthy", Times: failTimes}}
	n := startNode(t, sc, simnode.Options{})
	rpc := aktsolana.NewRPC(aktsolana.RPCOptions{URL: n.SolanaURL, Timeout: 10 * time.Second, Concurrency: workers, BlockConcurrency: workers})
	ctx := t.Context()

	stop := make(chan struct{})
	mutated := make(chan int)
	go func() {
		count := 0
		defer func() { mutated <- count }()
		for {
			select {
			case <-stop:
				return
			default:
			}
			n.Mutate(func(s *simnode.Scenario) {
				if s.Solana.Slot == finalSlot {
					s.Solana.Slot = finalSlot + 5
				} else {
					s.Solana.Slot = finalSlot
				}
			})
			count++
		}
	}()

	var failed atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				if _, err := rpc.GenesisHash(ctx); err != nil {
					assert.ErrorIs(t, err, aktsolana.ErrUnavailable)
					failed.Add(1)
				}

				slot, accts, err := rpc.MultipleAccounts(ctx, []solana.PublicKey{f.status})
				if assert.NoError(t, err) {
					assert.Contains(t, []uint64{finalSlot, finalSlot + 5}, slot)
					assert.NotNil(t, accts[0])
				}

				txs := 0
				assert.NoError(t, rpc.StreamBlock(ctx, resolveSlot, func(aktsolana.Tx) error {
					txs++
					return nil
				}))
				assert.Equal(t, 3, txs)
			}
		}()
	}
	wg.Wait()
	close(stop)

	require.Positive(t, <-mutated)
	require.EqualValues(t, failTimes, failed.Load())
	require.Equal(t, workers*rounds, n.Calls("getGenesisHash"))
	require.Equal(t, workers*rounds, n.Calls("getBlock"))
}

// chainText renders the whole wrapped chain, since ErrUnavailable keeps only its own text.
func chainText(err error) string {
	var texts []string
	queue := []error{err}
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		texts = append(texts, e.Error())
		switch u := e.(type) {
		case interface{ Unwrap() []error }:
			queue = append(queue, u.Unwrap()...)
		case interface{ Unwrap() error }:
			if next := u.Unwrap(); next != nil {
				queue = append(queue, next)
			}
		}
	}

	return strings.Join(texts, " | ")
}

func TestScenarioRoundTripsJSON(t *testing.T) {
	sc := newFixture(t).scenario()
	sc.Passes = 3
	sc.Solana.Failures = map[string]simnode.Failure{"getBlock": {Code: -32004, Message: "x", Times: 1}}

	raw, err := json.Marshal(sc)
	require.NoError(t, err)

	var back simnode.Scenario
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Equal(t, sc, back)

	require.NotNil(t, simnode.Scenarios())
}

func TestCloseStopsServing(t *testing.T) {
	n, err := simnode.Start(context.Background(), newFixture(t).scenario(), simnode.Options{})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(n.SolanaURL, "http://127.0.0.1:"))

	n.Close()
	n.Close()

	resp, err := http.Post(n.SolanaURL, "application/json", strings.NewReader(`{}`)) //nolint:noctx // test
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	require.Error(t, err)
}
