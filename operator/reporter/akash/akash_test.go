package akash

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// start is a whole second, so the freshness bounds are exact.
var start = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

const (
	testMaxAge = 60 * time.Second
	// head is the latest height of the named scenarios.
	head = int64(100)

	pathState     = "/akash.sink.v1.Query/State"
	pathParams    = "/akash.sink.v1.Query/Params"
	pathReporters = "/akash.sink.v1.Query/Reporters"
	pathTransfer  = "/akash.sink.v1.Query/Transfer"

	codespaceSDK = "sdk"
)

func startNode(t *testing.T, sc simnode.Scenario, opts simnode.Options) (*simnode.Node, *clock.Fake) {
	t.Helper()

	fake := clock.NewFake(start)
	opts.Clock = fake

	n, err := simnode.Start(t.Context(), sc, opts)
	require.NoError(t, err)
	t.Cleanup(n.Close)

	return n, fake
}

func options(n *simnode.Node, c clock.Clock) Options {
	return Options{
		Node:           n.CometURL,
		GRPCAddr:       n.GRPCAddr,
		ChainID:        simnode.ChainID,
		MaxAge:         testMaxAge,
		RequestTimeout: 5 * time.Second,
		Concurrency:    8,
		Clock:          c,
	}
}

func dial(t *testing.T, opts Options) *Client {
	t.Helper()

	c, err := Dial(t.Context(), opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	return c
}

func clean() simnode.Scenario { return simnode.Scenarios()["clean"]() }

func owner(t *testing.T, b byte) (string, []byte) {
	t.Helper()

	raw := bytes.Repeat([]byte{b}, 20)
	s, err := bech32.ConvertAndEncode("akash", raw)
	require.NoError(t, err)

	return s, raw
}

func newID(t *testing.T, o byte, height int64, sequence uint64) sinkid.ID {
	t.Helper()

	s, raw := owner(t, o)

	return sinkid.ID{Owner: s, OwnerBytes: raw, Height: height, Sequence: sequence}
}

// record is a valid transfer under the scenario's current policy.
func record(sc simnode.Scenario, id sinkid.ID, st sinkv1.Status, hold sinkv1.Hold) simnode.Transfer {
	return simnode.Transfer{Transfer: sinkv1.TransferState{
		ID:         id.Proto(),
		Data:       sinkv1.RecordData{Dest: solana.PublicKeyFromBytes(bytes.Repeat([]byte{id.OwnerBytes[0]}, 32)).String(), Amount: sdk.NewInt64Coin("uakt", 1_000)},
		Status:     st,
		Settlement: sinkv1.SettlementData{Round: 1, Policy: sc.Akash.Policies[len(sc.Akash.Policies)-1], Hold: hold},
	}}
}

// voteTx is an unordered vote the simulator admits; it decodes transactions but does not verify signatures.
func voteTx(t *testing.T, id sinkv1.ID, timeout time.Time) []byte {
	t.Helper()

	cfg := simnode.TxConfig()
	b := cfg.NewTxBuilder()
	require.NoError(t, b.SetMsgs(&sinkv1.MsgReportOutcome{
		Reporter:      simnode.Reporter,
		ID:            id,
		PolicyVersion: 1,
		Round:         1,
		Observation:   sinkv1.Observation{Outcome: sinkv1.OutcomeVoided, Slot: 9, TransactionSignature: bytes.Repeat([]byte{1}, 64)},
	}))
	b.SetMemo("AKT sink reporter")
	b.SetUnordered(true)
	b.SetTimeoutTimestamp(timeout)
	b.SetGasLimit(500_000)
	b.SetFeeAmount(sdk.NewCoins(sdk.NewInt64Coin("uakt", 5_000)))
	require.NoError(t, b.SetSignatures(signing.SignatureV2{
		PubKey: secp256k1.GenPrivKeyFromSecret([]byte("akash client test")).PubKey(),
		Data:   &signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_DIRECT, Signature: []byte("unverified")},
	}))

	raw, err := cfg.TxEncoder()(b.GetTx())
	require.NoError(t, err)

	return raw
}

func ids(list ...sinkid.ID) []string {
	out := make([]string, len(list))
	for i, id := range list {
		out[i] = id.String()
	}

	return out
}

func TestAkashFromTS(t *testing.T) {
	t.Run("queries sink state and transfer at one committed height; rejects stale/wrong-chain RPC", func(t *testing.T) {
		sc := clean()
		// CometBFT advertises 100 before the application commits it.
		sc.Akash.AppHeight = head - 1
		refund := newID(t, 0x31, 20, 1)
		pending := record(sc, refund, sinkv1.StatusRefundPending, sinkv1.HoldNone)
		pending.Transfer.Settlement.ChallengeEnd = start.Add(time.Hour).Unix()
		sc.Akash.Transfers = append(sc.Akash.Transfers, pending)
		first, err := sinkid.FromProto(sc.Akash.Transfers[0].Transfer.ID)
		require.NoError(t, err)

		n, fake := startNode(t, sc, simnode.Options{})
		c := dial(t, options(n, fake))
		latest := func() (*State, error) {
			fake.Advance(time.Second)
			return c.Latest(t.Context())
		}

		s, err := c.Latest(t.Context())
		require.NoError(t, err)
		require.Equal(t, head-1, s.Height)
		require.Equal(t, start.Add(-time.Second-6*time.Second), s.Time.UTC())
		require.EqualValues(t, 1, s.Policy.Version)
		require.EqualValues(t, 20_000, s.Epoch)
		require.Equal(t, uint64(20_000), s.RefundUsage.Epoch)
		require.True(t, s.Params.Enabled)
		require.NotNil(t, s.RefundHead)
		require.Equal(t, refund.String(), s.RefundHead.String())

		transfer, err := c.Transfer(t.Context(), s.Height, first)
		require.NoError(t, err)
		require.Equal(t, sc.Akash.Transfers[0].Transfer, transfer)
		for _, path := range []string{pathState, pathParams, pathReporters, pathTransfer} {
			require.Equal(t, 1, n.Calls(path), path)
		}

		// gRPC NotFound is a missing record; pruned and future heights mean an unavailable node.
		_, err = c.Transfer(t.Context(), s.Height, newID(t, 0x77, 9, 9))
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.EqualError(t, err, "Sink record not found")
		for _, h := range []int64{head - 10, head + 1} {
			_, err = c.Transfer(t.Context(), h, first)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrRecordNotFound)
		}

		number, err := c.AccountNumber(t.Context(), sdk.MustAccAddressFromBech32(simnode.Reporter))
		require.NoError(t, err)
		require.EqualValues(t, 7, number)
		_, err = c.AccountNumber(t.Context(), bytes.Repeat([]byte{0x99}, 20))
		require.EqualError(t, err, "reporter account not found")

		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.State.RefundUsage.Epoch-- })
		_, err = latest()
		require.EqualError(t, err, "quota usage epoch mismatch")

		n.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.State.Epoch, sc.Akash.State.SendUsage.Epoch, sc.Akash.State.RefundUsage.Epoch = 0, 0, 0
		})
		s, err = latest()
		require.NoError(t, err)
		require.Zero(t, s.Epoch)

		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.State.RefundUsage.Amount = sdk.NewInt64Coin("uakt", 1) })
		_, err = latest()
		require.EqualError(t, err, "usage before quota epoch starts")
		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.State.RefundUsage.Amount = sdk.NewInt64Coin("uakt", 0) })

		n.Mutate(func(sc *simnode.Scenario) {
			e := uint64(math.MaxInt64) + 1
			sc.Akash.State.Epoch, sc.Akash.State.SendUsage.Epoch, sc.Akash.State.RefundUsage.Epoch = e, e, e
		})
		_, err = latest()
		require.EqualError(t, err, "invalid quota epoch")
		n.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.State.Epoch, sc.Akash.State.SendUsage.Epoch, sc.Akash.State.RefundUsage.Epoch = 7, 7, 7
		})

		policies := sc.Akash.Policies
		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.Policies = nil })
		_, err = latest()
		require.EqualError(t, err, "missing current reporter policy")
		n.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.Policies = []sinkv1.ReporterPolicy{{Members: policies[0].Members}}
		})
		_, err = latest()
		require.EqualError(t, err, "missing sink state")
		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.Policies = policies })

		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.EchoOffset = 1 })
		_, err = latest()
		require.EqualError(t, err, "Akash query height mismatch")
		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.EchoOffset = 0 })

		s, err = latest()
		require.NoError(t, err)
		require.EqualValues(t, 7, s.Epoch)

		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.ChainID = "other-chain" })
		_, err = latest()
		require.EqualError(t, err, "wrong Akash chain")
		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.ChainID = simnode.ChainID })

		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.CatchingUp = true })
		_, err = latest()
		require.EqualError(t, err, "Akash node is syncing")
		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.CatchingUp = false })

		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.BlockTimes[head] = -time.Hour })
		_, err = latest()
		require.EqualError(t, err, "stale Akash node")

		// The committed height's own block must be fresh too.
		n.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.BlockTimes[head] = 0
			sc.Akash.BlockTimes[head-1] = -2 * time.Minute
		})
		_, err = latest()
		require.EqualError(t, err, "stale Akash committed state")

		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.AppHeight = 1 })
		_, err = latest()
		require.EqualError(t, err, "Akash node has no queryable committed state")
	})

	t.Run("chronological discovery reuses the server's last-page cursor and finds later earlier-sorting owners", func(t *testing.T) {
		sc := clean()
		first, second, third := newID(t, 0x09, 10, 1), newID(t, 0x09, 10, 2), newID(t, 0x09, 10, 3)
		later := newID(t, 0x01, 11, 1)
		sc.Akash.Transfers = []simnode.Transfer{
			record(sc, first, sinkv1.StatusMinted, sinkv1.HoldNone),
			record(sc, second, sinkv1.StatusMinted, sinkv1.HoldNone),
			record(sc, third, sinkv1.StatusMinted, sinkv1.HoldNone),
		}
		laterRecord := record(sc, later, sinkv1.StatusMinted, sinkv1.HoldNone)

		n, fake := startNode(t, sc, simnode.Options{})
		c := dial(t, options(n, fake))

		begin, err := c.Audit(t.Context(), head, nil, 2)
		require.NoError(t, err)
		require.Equal(t, ids(first, second), ids(begin.IDs...))
		require.True(t, begin.HasMore)
		require.Equal(t, CursorNext, begin.Cursor.Kind)
		require.NotEmpty(t, begin.Cursor.Key)
		require.Equal(t, second.String(), begin.Cursor.LastID.String())

		end, err := c.Audit(t.Context(), head, begin.Cursor, 2)
		require.NoError(t, err)
		require.Equal(t, ids(third), ids(end.IDs...))
		require.False(t, end.HasMore)
		require.Equal(t, &AuditCursor{Kind: CursorTail, Key: begin.Cursor.Key, LastID: third}, end.Cursor)

		again, err := c.Audit(t.Context(), head, end.Cursor, 2)
		require.NoError(t, err)
		require.Empty(t, again.IDs)

		n.Mutate(func(sc *simnode.Scenario) { sc.Akash.Transfers = append(sc.Akash.Transfers, laterRecord) })
		next, err := c.Audit(t.Context(), head, end.Cursor, 2)
		require.NoError(t, err)
		require.Equal(t, ids(later), ids(next.IDs...))
		require.Equal(t, later.String(), next.Cursor.LastID.String())

		n.Mutate(func(sc *simnode.Scenario) {
			sc.Akash.Transfers = []simnode.Transfer{sc.Akash.Transfers[0], sc.Akash.Transfers[1], laterRecord}
		})
		_, err = c.Audit(t.Context(), head, end.Cursor, 2)
		require.ErrorContains(t, err, "checkpoint disappeared")
	})

	t.Run("native pages preserve fixed height, reject duplicates, stalled cursors and pruned state", func(t *testing.T) {
		a, b := newID(t, 0x41, 3, 1), newID(t, 0x41, 3, 2)
		var keys [][]byte
		result, err := collectPages(t.Context(), func(_ context.Context, key []byte) (page, error) {
			keys = append(keys, key)
			if len(key) > 0 {
				return page{ids: []sinkid.ID{b}}, nil
			}
			return page{ids: []sinkid.ID{a}, nextKey: []byte{2}}, nil
		})
		require.NoError(t, err)
		require.Len(t, result, 2)
		require.Equal(t, [][]byte{nil, {2}}, keys)

		tokens := [][]byte{{255}, {1}, nil}
		calls := 0
		counting := func(next func(int) []byte) func(context.Context, []byte) (page, error) {
			calls = 0
			return func(context.Context, []byte) (page, error) {
				calls++
				return page{ids: []sinkid.ID{newID(t, 0x42, 3, uint64(calls))}, nextKey: next(calls)}, nil //nolint:gosec // small
			}
		}
		unordered, err := collectPages(t.Context(), counting(func(i int) []byte { return tokens[i-1] }))
		require.NoError(t, err)
		require.Len(t, unordered, 3, "opaque cursor bytes need not increase")

		_, err = collectPages(t.Context(), counting(func(i int) []byte { return tokens[(i-1)%2] }))
		require.ErrorContains(t, err, "cursor did not advance")

		_, err = collectPages(t.Context(), func(context.Context, []byte) (page, error) {
			return page{ids: []sinkid.ID{a}, nextKey: []byte{2}}, nil
		})
		require.ErrorContains(t, err, "duplicate")

		_, err = collectPages(t.Context(), func(context.Context, []byte) (page, error) {
			return page{nextKey: []byte{2}}, nil
		})
		require.ErrorContains(t, err, "advance")

		// Every page is pinned to the listing height; a pruned height fails without a fallback.
		n, fake := startNode(t, clean(), simnode.Options{})
		c := dial(t, options(n, fake))
		open, err := c.ListOpen(t.Context(), head, 2)
		require.NoError(t, err)
		require.Len(t, open, 5)
		_, err = c.ListOpen(t.Context(), head-10, 2)
		require.ErrorContains(t, err, "failed to load state")
	})

	t.Run("fixed-height listings return every record past the old 100,000 bound", func(t *testing.T) {
		const total = 100_001

		rows := make([]sinkid.ID, total)
		for i := range rows {
			rows[i] = newID(t, 0x05, 7, uint64(i+1)) //nolint:gosec // positive
		}
		read := func(_ context.Context, key []byte) (page, error) {
			from := 0
			if len(key) > 0 {
				from = int(binary.BigEndian.Uint32(key))
			}
			to := min(from+500, total)
			p := page{ids: rows[from:to]}
			if to < total {
				p.nextKey = binary.BigEndian.AppendUint32(nil, uint32(to)) //nolint:gosec // small
			}
			return p, nil
		}
		history, err := collectPages(t.Context(), read)
		require.NoError(t, err)
		require.Len(t, history, total)
		require.Equal(t, rows[total-1].String(), history[total-1].String())

		sc := clean()
		sc.Akash.Transfers = make([]simnode.Transfer, total)
		for i, id := range rows {
			sc.Akash.Transfers[i] = record(sc, id, sinkv1.StatusTransferPending, sinkv1.HoldNone)
		}
		n, fake := startNode(t, sc, simnode.Options{})
		c := dial(t, options(n, fake))
		open, err := c.ListOpen(t.Context(), head, 500)
		require.NoError(t, err)
		require.Len(t, open, total)
		require.Equal(t, rows[total-1].String(), open[total-1].String())
	})
}

// TestListingChecks covers the TS listing texts that a well-behaved node cannot produce.
func TestListingChecks(t *testing.T) {
	sc := clean()
	pending := record(sc, newID(t, 0x51, 4, 1), sinkv1.StatusTransferPending, sinkv1.HoldNone).Transfer
	held := record(sc, newID(t, 0x52, 4, 1), sinkv1.StatusMinted, sinkv1.HoldConflict).Transfer
	more := &sdkquery.PageResponse{NextKey: []byte{1}}

	p, err := decodePage(source{status: sinkv1.StatusTransferPending}, 1, []sinkv1.TransferState{pending}, more)
	require.NoError(t, err)
	require.Equal(t, []byte{1}, p.nextKey)

	_, err = decodePage(source{status: sinkv1.StatusTransferPending}, 1, []sinkv1.TransferState{pending}, nil)
	require.EqualError(t, err, "invalid transfers page")
	_, err = decodePage(source{}, 1, []sinkv1.TransferState{pending, held}, more)
	require.EqualError(t, err, "invalid transfers page")
	_, err = decodePage(source{disputes: true}, 1, nil, nil)
	require.EqualError(t, err, "invalid disputes page")
	_, err = decodePage(source{status: sinkv1.StatusVoided}, 1, []sinkv1.TransferState{pending}, more)
	require.EqualError(t, err, "status filter mismatch")
	_, err = decodePage(source{disputes: true}, 1, []sinkv1.TransferState{pending}, more)
	require.EqualError(t, err, "unheld dispute")

	bare := pending
	bare.Settlement = sinkv1.SettlementData{}
	_, err = decodePage(source{}, 1, []sinkv1.TransferState{bare}, more)
	require.EqualError(t, err, "missing settlement")

	noncanonical := pending
	noncanonical.ID.Sequence = 0
	_, err = decodePage(source{}, 1, []sinkv1.TransferState{noncanonical}, more)
	require.Error(t, err)

	n, fake := startNode(t, clean(), simnode.Options{})
	c := dial(t, options(n, fake))
	_, err = c.ListOpen(t.Context(), head, 0)
	require.EqualError(t, err, "invalid page limit")

	// History must be chronological within a page and after the last checkpoint.
	a, b := newID(t, 0x61, 5, 1), newID(t, 0x61, 5, 2)
	_, err = discover(nil, page{ids: []sinkid.ID{b, a}})
	require.EqualError(t, err, "history is not chronological")
	_, err = discover(&AuditCursor{Kind: CursorNext, Key: []byte{1}, LastID: b}, page{ids: []sinkid.ID{a}})
	require.EqualError(t, err, "history is not chronological")
	_, err = discover(&AuditCursor{Kind: CursorNext, Key: []byte{1}, LastID: a}, page{})
	require.EqualError(t, err, "chronological checkpoint disappeared")
	_, err = discover(&AuditCursor{Kind: CursorNext, Key: []byte{1}, LastID: a}, page{ids: []sinkid.ID{b}, nextKey: []byte{1}})
	require.EqualError(t, err, "pagination cursor did not advance")
}

// TestBlockChecks covers the block texts that simnode, which serves its own chain ID at every height, cannot produce.
func TestBlockChecks(t *testing.T) {
	fake := clock.NewFake(start)
	c := &Client{chainID: simnode.ChainID, maxAge: testMaxAge, clock: fake}
	header := cmttypes.Header{ChainID: simnode.ChainID, Height: 42, Time: start}

	require.NoError(t, c.checkBlock(42, header))
	require.EqualError(t, c.checkBlock(43, header), "Akash block height mismatch")
	header.ChainID = "other-chain"
	require.EqualError(t, c.checkBlock(42, header), "wrong Akash block chain")
	header.ChainID = simnode.ChainID
	header.Time = start.Add(-61 * time.Second)
	require.EqualError(t, c.checkBlock(42, header), "stale Akash committed state")
}

func TestDialRejectsDifferentNodes(t *testing.T) {
	a, fake := startNode(t, clean(), simnode.Options{})
	other := clean()
	other.Akash.NodeID = "ffffffffffffffffffffffffffffffffffffffff"
	b, _ := startNode(t, other, simnode.Options{})

	opts := options(a, fake)
	opts.GRPCAddr = b.GRPCAddr
	_, err := Dial(t.Context(), opts)
	require.EqualError(t, err, "Akash RPC and gRPC endpoints are different nodes")
}

func TestTrackerRefreshAtMostOncePerInterval(t *testing.T) {
	n, fake := startNode(t, clean(), simnode.Options{AkashLatency: 20 * time.Millisecond})
	c := dial(t, options(n, fake))
	before := n.Calls("status")

	var wg sync.WaitGroup
	states := make([]*State, 100)
	errs := make([]error, 100)
	for i := range states {
		wg.Go(func() { states[i], errs[i] = c.Latest(t.Context()) })
	}
	wg.Wait()

	for i := range states {
		require.NoError(t, errs[i])
		require.Same(t, states[0], states[i])
	}
	require.Equal(t, 1, n.Calls("status")-before)

	// Once the interval passes, the next call refreshes.
	fake.Advance(time.Second)
	_, err := c.Latest(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, n.Calls("status")-before)
}

// hungSink answers no transfer query: each signals entered, then waits until its call ends.
type hungSink struct {
	sinkv1.UnimplementedQueryServer
	entered chan struct{}
}

func (s *hungSink) Transfer(ctx context.Context, _ *sinkv1.QueryTransferRequest) (*sinkv1.QueryTransferResponse, error) {
	s.entered <- struct{}{}
	<-ctx.Done()
	return nil, status.FromContextError(ctx.Err()).Err()
}

func TestShutdownCancelsQueuedReads(t *testing.T) {
	n, fake := startNode(t, clean(), simnode.Options{})
	opts := options(n, fake)
	opts.Concurrency = 2
	opts.RequestTimeout = 500 * time.Millisecond
	c := dial(t, opts)

	// Ten times the concurrency limit: two calls run, the others wait for a permit.
	const calls = 20

	// The sink queries now reach a node that never answers.
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer(grpc.ForceServerCodec(codec.NewProtoCodec(c.registry).GRPCCodec()))
	sink := &hungSink{entered: make(chan struct{}, calls)}
	sinkv1.RegisterQueryServer(srv, sink)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient(l.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(codec.NewProtoCodec(c.registry).GRPCCodec())),
		grpc.WithUnaryInterceptor(c.intercept),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.sink = sinkv1.NewQueryClient(conn)

	stop, shutdown := context.WithCancel(t.Context())
	ctx := WithStop(context.WithoutCancel(stop), stop)
	id := newID(t, 1, 10, 1)
	errs := make(chan error, calls)
	for range calls {
		go func() {
			_, err := c.Transfer(ctx, head, id)
			errs <- err
		}()
	}
	// Both permits are held once two reads reach the node; the others wait for a permit.
	for range opts.Concurrency {
		select {
		case <-sink.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the reads holding the permits never reached the node")
		}
	}

	began := time.Now()
	shutdown()
	timedOut, cancelled := 0, 0
	for range calls {
		select {
		case err := <-errs:
			switch {
			case status.Code(errors.Unwrap(err)) == codes.DeadlineExceeded:
				timedOut++
			case errors.Is(err, context.Canceled):
				cancelled++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the queued reads did not end")
		}
	}

	require.Less(t, time.Since(began), 2*opts.RequestTimeout, "shutdown takes about one request timeout")
	require.Equal(t, opts.Concurrency, timedOut, "the reads in flight finish or time out")
	require.Equal(t, calls-opts.Concurrency, cancelled, "the reads waiting for a permit are cancelled")
}

func TestBlockMaxGas(t *testing.T) {
	n, fake := startNode(t, clean(), simnode.Options{})
	got, err := dial(t, options(n, fake)).BlockMaxGas(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(-1), got, "unlimited")

	sc := clean()
	sc.Akash.BlockMaxGas = 5_000_000
	n, fake = startNode(t, sc, simnode.Options{})
	got, err = dial(t, options(n, fake)).BlockMaxGas(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(5_000_000), got)
}

func TestTrackerCachesFailure(t *testing.T) {
	n, fake := startNode(t, simnode.Scenarios()["stale-akash"](), simnode.Options{})
	c := dial(t, options(n, fake))
	before := n.Calls("status")

	_, err := c.Latest(t.Context())
	require.EqualError(t, err, "stale Akash node")
	fake.Advance(999 * time.Millisecond)
	_, err = c.Latest(t.Context())
	require.EqualError(t, err, "stale Akash node")
	require.Equal(t, 1, n.Calls("status")-before)
}

func TestTrackerAcceptsLowerHeight(t *testing.T) {
	sc := clean()
	sc.Akash.Height = 10
	sc.Akash.BlockTimes = map[int64]time.Duration{8: -13 * time.Second, 9: -7 * time.Second, 10: -time.Second}
	n, fake := startNode(t, sc, simnode.Options{})
	c := dial(t, options(n, fake))

	s, err := c.Latest(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 10, s.Height)

	n.SetLag(1)
	fake.Advance(time.Second)
	lower, err := c.Latest(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 9, lower.Height)
	require.Equal(t, start.Add(-7*time.Second), lower.Time.UTC())
	require.EqualValues(t, 1, lower.Policy.Version)
}

func TestFreshnessWindow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset time.Duration
		err    string
	}{
		{"ten seconds ahead", 10 * time.Second, ""},
		{"eleven seconds ahead", 11 * time.Second, "stale Akash node"},
		{"max-age old", -testMaxAge, ""},
		{"past max-age", -testMaxAge - time.Second, "stale Akash node"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := clean()
			sc.Akash.BlockTimes[head] = tc.offset
			n, fake := startNode(t, sc, simnode.Options{})
			c := dial(t, options(n, fake))

			s, err := c.Latest(t.Context())
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, start.Add(tc.offset), s.Time.UTC())
		})
	}
}

func TestHeightEchoMismatch(t *testing.T) {
	sc := clean()
	sc.Akash.EchoOffset = 1
	first, err := sinkid.FromProto(sc.Akash.Transfers[0].Transfer.ID)
	require.NoError(t, err)
	n, fake := startNode(t, sc, simnode.Options{})
	c := dial(t, options(n, fake))

	_, err = c.Latest(t.Context())
	require.EqualError(t, err, "Akash query height mismatch")
	_, err = c.Transfer(t.Context(), head, first)
	require.EqualError(t, err, "Akash transfer query height mismatch")
	_, err = c.Audit(t.Context(), head, nil, 10)
	require.EqualError(t, err, "Akash transfers query height mismatch")
}

func TestMergeOrder(t *testing.T) {
	sc := clean()
	// heldRefund is both disputed and refund-pending; heldPending is both disputed and transfer-pending.
	heldRefund := newID(t, 0x21, 12, 1)
	heldPending := newID(t, 0x22, 11, 1)
	refund := newID(t, 0x23, 3, 1)
	voided := newID(t, 0x24, 2, 1)
	pendingA, pendingB := newID(t, 0x25, 1, 1), newID(t, 0x26, 1, 1)
	sc.Akash.Transfers = []simnode.Transfer{
		record(sc, pendingB, sinkv1.StatusTransferPending, sinkv1.HoldNone),
		record(sc, voided, sinkv1.StatusVoided, sinkv1.HoldNone),
		record(sc, heldRefund, sinkv1.StatusRefundPending, sinkv1.HoldVeto),
		record(sc, refund, sinkv1.StatusRefundPending, sinkv1.HoldNone),
		record(sc, newID(t, 0x27, 1, 1), sinkv1.StatusMinted, sinkv1.HoldNone),
		record(sc, pendingA, sinkv1.StatusTransferPending, sinkv1.HoldNone),
		record(sc, heldPending, sinkv1.StatusTransferPending, sinkv1.HoldConflict),
	}
	n, fake := startNode(t, sc, simnode.Options{})
	c := dial(t, options(n, fake))

	open, err := c.ListOpen(t.Context(), head, 1)
	require.NoError(t, err)
	require.Equal(t, ids(heldPending, heldRefund, refund, voided, pendingA, pendingB), ids(open...))
}

func TestPlaintextForLoopback(t *testing.T) {
	n, fake := startNode(t, clean(), simnode.Options{})
	opts := options(n, fake)
	opts.GRPCInsecure = false
	c := dial(t, opts)

	_, err := c.Latest(t.Context())
	require.NoError(t, err)

	for addr, want := range map[string]bool{
		"127.0.0.1:9090":    true,
		"127.1.2.3:9090":    true,
		"localhost:9090":    true,
		"[::1]:9090":        true,
		"10.0.0.1:9090":     false,
		"grpc.example:9090": false,
	} {
		require.Equal(t, want, plaintext(addr, false), addr)
		require.True(t, plaintext(addr, true), addr)
	}
}

func TestRedactURL(t *testing.T) {
	for raw, origin := range map[string]string{
		"http://127.0.0.1:26657/SECRET?token=SECRET": "http://127.0.0.1:26657",
		"http://[::1]:26657/SECRET":                  "http://[::1]:26657",
		// Unparsable, or no host: the raw URL never stays in the text.
		"http://SECRET:%Q/": "invalid URL",
		"http:///SECRET":    "invalid URL",
	} {
		// CometBFT renders the transport error into its own text.
		err := redactURL(fmt.Errorf("post failed: %w", &url.Error{Op: "Post", URL: raw, Err: context.Canceled}))
		require.EqualError(t, err, `post failed: Post "`+origin+`": context canceled`, raw)
		require.ErrorIs(t, err, context.Canceled, raw)
		var ue *url.Error
		require.False(t, errors.As(err, &ue), "the raw URL stays reachable: %s", raw)
	}

	// Only a cause known to hold no URL text keeps its text and stays reachable.
	for _, tc := range []struct {
		cause     error
		text      string
		reachable bool
	}{
		{context.DeadlineExceeded, "context deadline exceeded", true},
		{&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}, "dial tcp: connect: connection refused", true},
		{&net.DNSError{Err: "no such host", Name: "node.example"}, "lookup node.example: no such host", true},
		{&tls.CertificateVerificationError{Err: errors.New("x509: unknown authority")}, "tls: failed to verify certificate: x509: unknown authority", true},
		{errors.New(`failed to parse Location header "/SECRET"`), "HTTP request failed", false},
	} {
		err := redactURL(fmt.Errorf("post failed: %w", &url.Error{Op: "Post", URL: "http://node.example/SECRET", Err: tc.cause}))
		require.EqualError(t, err, `post failed: Post "http://node.example": `+tc.text)
		require.Equal(t, tc.reachable, errors.Is(err, tc.cause), tc.text)
	}

	// Of the text around the url.Error, only a prefix stays, with the URL cut down.
	ue := &url.Error{Op: "Post", URL: "http://node.example/SECRET", Err: context.Canceled}
	require.EqualError(t, redactURL(fmt.Errorf("query %q: %w", ue.URL, ue)), `query "http://node.example": Post "http://node.example": context canceled`)
	require.EqualError(t, redactURL(fmt.Errorf("%w; retried %q", ue, "/SECRET")), `Post "http://node.example": context canceled`)

	other := errors.New("RPC error -32603 - Internal error")
	require.Same(t, other, redactURL(other))
	require.NoError(t, redactURL(nil))
}

func TestServerSentURLTextRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/SECRET/%zz") // net/http quotes a Location it cannot parse in its error
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	_, err := Dial(t.Context(), Options{Node: server.URL + "/SECRET", GRPCAddr: "127.0.0.1:1", RequestTimeout: 5 * time.Second})
	for e := err; e != nil; e = errors.Unwrap(e) {
		require.NotContains(t, e.Error(), "SECRET")
	}
	require.EqualError(t, err, `post failed: Post "`+server.URL+`": HTTP request failed`)
}

func TestBroadcastInCache(t *testing.T) {
	sc := clean()
	n, fake := startNode(t, sc, simnode.Options{})
	c := dial(t, options(n, fake))
	raw := voteTx(t, sc.Akash.Transfers[0].Transfer.ID, start.Add(time.Minute))
	hash := sha256.Sum256(raw)

	first, err := c.BroadcastSync(t.Context(), raw)
	require.NoError(t, err)
	require.Zero(t, first.Code, first.Log)
	require.False(t, first.InCache)
	require.Equal(t, hash[:], first.Hash)

	second, err := c.BroadcastSync(t.Context(), raw)
	require.NoError(t, err)
	require.Equal(t, BroadcastResult{Hash: hash[:], InCache: true}, second)

	// A CheckTx rejection is a result, not an error.
	n.Mutate(func(sc *simnode.Scenario) {
		sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultReject, Code: 5, Codespace: codespaceSDK, Log: "insufficient funds"}}
	})
	rejected, err := c.BroadcastSync(t.Context(), voteTx(t, sc.Akash.Transfers[0].Transfer.ID, start.Add(time.Minute+time.Millisecond)))
	require.NoError(t, err)
	require.Equal(t, uint32(5), rejected.Code)
	require.Equal(t, codespaceSDK, rejected.Codespace)
	require.Equal(t, "insufficient funds", rejected.Log)

	// A lost connection is a transport error.
	n.Mutate(func(sc *simnode.Scenario) {
		sc.Akash.BroadcastFaults = []simnode.BroadcastFault{{Kind: simnode.FaultTransport}}
	})
	_, err = c.BroadcastSync(t.Context(), voteTx(t, sc.Akash.Transfers[0].Transfer.ID, start.Add(time.Minute+2*time.Millisecond)))
	require.Error(t, err)
}

func TestTxResultCodespace(t *testing.T) {
	sc := clean()
	sc.Akash.Deliveries = []simnode.Delivery{
		{Code: 18, Codespace: codespaceSDK, Log: "failed to add unordered nonce: sender has already used timeout"},
		{Code: 18, Codespace: "sink", Log: "other module error"},
	}
	n, fake := startNode(t, sc, simnode.Options{})
	c := dial(t, options(n, fake))
	require.True(t, c.TxIndexEnabled())

	id := sc.Akash.Transfers[0].Transfer.ID
	sdkTx, err := c.BroadcastSync(t.Context(), voteTx(t, id, start.Add(time.Minute)))
	require.NoError(t, err)
	require.Zero(t, sdkTx.Code, sdkTx.Log)
	sinkTx, err := c.BroadcastSync(t.Context(), voteTx(t, id, start.Add(time.Minute+time.Millisecond)))
	require.NoError(t, err)
	require.Zero(t, sinkTx.Code, sinkTx.Log)

	// Not yet included: no result and no error.
	missing, err := c.Tx(t.Context(), sdkTx.Hash)
	require.NoError(t, err)
	require.Nil(t, missing)

	n.CommitBlock()

	got, err := c.Tx(t.Context(), sdkTx.Hash)
	require.NoError(t, err)
	require.Equal(t, &TxResult{Height: head + 1, Code: 18, Codespace: codespaceSDK, Log: "failed to add unordered nonce: sender has already used timeout"}, got)

	got, err = c.Tx(t.Context(), sinkTx.Hash)
	require.NoError(t, err)
	require.Equal(t, &TxResult{Height: head + 1, Code: 18, Codespace: "sink", Log: "other module error"}, got)

	// A node without a transaction index says so at Dial, and its tx lookups fail.
	noIndex := clean()
	noIndex.Akash.NoTxIndex = true
	m, mfake := startNode(t, noIndex, simnode.Options{})
	d := dial(t, options(m, mfake))
	require.False(t, d.TxIndexEnabled())
	_, err = d.Tx(t.Context(), sdkTx.Hash)
	require.ErrorContains(t, err, "transaction indexing is disabled")
}

func TestSimulate(t *testing.T) {
	sc := clean()
	id := sc.Akash.Transfers[0].Transfer.ID
	sc.Akash.SimFailures = []simnode.SimFailure{{ID: sc.Akash.Transfers[1].Transfer.ID.Owner + "/11/1", Error: "injected failure"}}
	n, fake := startNode(t, sc, simnode.Options{})
	c := dial(t, options(n, fake))

	gas, err := c.Simulate(t.Context(), voteTx(t, id, start.Add(time.Minute)))
	require.NoError(t, err)
	require.Positive(t, gas)

	_, err = c.Simulate(t.Context(), voteTx(t, sc.Akash.Transfers[1].Transfer.ID, start.Add(time.Minute)))
	require.ErrorContains(t, err, "message index: 0")
	require.ErrorContains(t, err, "injected failure")
}
