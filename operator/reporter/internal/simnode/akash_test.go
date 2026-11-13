package simnode_test

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

const (
	akashChain  = "simnode-test-1"
	akashNodeID = "0123456789abcdef0123456789abcdef01234567"
	latest      = int64(10)
	// latestOffset is the latest block's time relative to Start.
	latestOffset = -2 * time.Second
	challenge    = 72 * time.Hour

	typeVote   = "/akash.sink.v1.MsgReportOutcome"
	typeVeto   = "/akash.sink.v1.MsgVetoVoid"
	typeRefund = "/akash.sink.v1.MsgFinalizeRefund"
)

var threshold = sdkmath.LegacyMustNewDecFromStr("0.67")

// chain is an Akash scenario: one reporter of a three-member policy and transfers in every state a message acts on.
type chain struct {
	priv     *secp256k1.PrivKey
	reporter string
	policy   sinkv1.ReporterPolicy
	// vote and vote2 are transfer-pending; veto is refund-pending inside its challenge window; refundA and refundB are
	// refund-pending past it, refundA first in the queue; held is a held dispute.
	vote, vote2, veto, refundA, refundB, held sinkid.ID
}

func bech(t *testing.T, raw []byte) string {
	t.Helper()

	s, err := bech32.ConvertAndEncode("akash", raw)
	require.NoError(t, err)

	return s
}

func sinkID(t *testing.T, owner byte, height int64, sequence uint64) sinkid.ID {
	t.Helper()

	raw := bytes.Repeat([]byte{owner}, 20)

	return sinkid.ID{Owner: bech(t, raw), OwnerBytes: raw, Height: height, Sequence: sequence}
}

func newChain(t *testing.T) chain {
	t.Helper()

	priv := secp256k1.GenPrivKeyFromSecret([]byte("simnode akash test reporter"))
	reporter := bech(t, priv.PubKey().Address())

	return chain{
		priv:     priv,
		reporter: reporter,
		policy: sinkv1.ReporterPolicy{
			Version:   1,
			Members:   []string{reporter, bech(t, bytes.Repeat([]byte{0xa1}, 20)), bech(t, bytes.Repeat([]byte{0xa2}, 20))},
			Threshold: threshold,
		},
		vote:    sinkID(t, 0x11, 3, 1),
		vote2:   sinkID(t, 0x12, 3, 2),
		veto:    sinkID(t, 0x13, 4, 1),
		refundA: sinkID(t, 0x14, 5, 1),
		refundB: sinkID(t, 0x15, 2, 1),
		held:    sinkID(t, 0x16, 1, 1),
	}
}

func uakt(amount int64) sdk.Coin { return sdk.NewInt64Coin("uakt", amount) }

func (c chain) transfer(id sinkid.ID, st sinkv1.Status, challengeEnd int64) sinkv1.TransferState {
	return sinkv1.TransferState{
		ID:     id.Proto(),
		Data:   sinkv1.RecordData{Dest: solana.PublicKeyFromBytes(bytes.Repeat([]byte{id.OwnerBytes[0]}, 32)).String(), Amount: uakt(1_000_000)},
		Status: st,
		Settlement: sinkv1.SettlementData{
			Round:             1,
			Policy:            c.policy,
			ChallengeEnd:      challengeEnd,
			VoidAuthorization: &sinkv1.VoidAuthorization{RequestedHeight: 2, RequestedRound: 1},
		},
	}
}

func (c chain) scenario(t *testing.T) simnode.Scenario {
	t.Helper()

	times := map[int64]time.Duration{}
	for h := int64(5); h <= latest; h++ {
		times[h] = latestOffset - time.Duration(latest-h)*6*time.Second
	}

	held := c.transfer(c.held, sinkv1.StatusTransferPending, 0)
	held.Settlement.Hold = sinkv1.HoldConflict

	return simnode.Scenario{Akash: &simnode.Akash{
		ChainID:    akashChain,
		NodeID:     akashNodeID,
		Height:     latest,
		BlockTimes: times,
		Params: sinkv1.Params{
			Enabled:                 true,
			MaxEpochRefund:          uakt(1_000_000_000),
			ReportersThreshold:      threshold,
			ReporterChallengePeriod: challenge,
			RefundsEnabled:          true,
		},
		Policies: []sinkv1.ReporterPolicy{c.policy},
		State: simnode.SinkState{
			Epoch:       7,
			SendUsage:   sinkv1.EpochUsage{Epoch: 7, Amount: uakt(0)},
			RefundUsage: sinkv1.EpochUsage{Epoch: 7, Amount: uakt(0)},
		},
		Transfers: []simnode.Transfer{
			{Transfer: c.transfer(c.vote, sinkv1.StatusTransferPending, 0)},
			{Transfer: c.transfer(c.vote2, sinkv1.StatusTransferPending, 0)},
			{Transfer: c.transfer(c.veto, sinkv1.StatusRefundPending, start.Add(time.Hour).Unix())},
			{Transfer: c.transfer(c.refundA, sinkv1.StatusRefundPending, start.Add(-time.Hour).Unix())},
			{Transfer: c.transfer(c.refundB, sinkv1.StatusRefundPending, start.Add(-30*time.Minute).Unix())},
			{Transfer: held},
		},
		Accounts: []simnode.AuthAccount{{Address: c.reporter, Number: 42}},
	}}
}

func (c chain) voteMsg(id sinkid.ID, round uint64) *sinkv1.MsgReportOutcome {
	return &sinkv1.MsgReportOutcome{
		Reporter:      c.reporter,
		ID:            id.Proto(),
		PolicyVersion: c.policy.Version,
		Round:         round,
		Observation:   sinkv1.Observation{Outcome: sinkv1.OutcomeVoided, Slot: 9, TransactionSignature: bytes.Repeat([]byte{1}, 64)},
	}
}

// voteKey is the execution key of a round-1 vote from voteMsg.
func voteKey(id sinkid.ID) string {
	return typeVote + "|" + id.String() + "|" + strconv.Itoa(int(sinkv1.OutcomeVoided)) + "|1|1"
}

// tx signs nothing: the simulator decodes transactions but does not verify signatures.
func (c chain) tx(t *testing.T, timeout time.Time, memo string, msgs ...sdk.Msg) []byte {
	t.Helper()

	cfg := simnode.TxConfig()
	b := cfg.NewTxBuilder()
	require.NoError(t, b.SetMsgs(msgs...))
	b.SetMemo(memo)
	b.SetUnordered(true)
	b.SetTimeoutTimestamp(timeout)
	b.SetGasLimit(500_000)
	b.SetFeeAmount(sdk.NewCoins(uakt(5_000)))
	require.NoError(t, b.SetSignatures(signing.SignatureV2{
		PubKey: c.priv.PubKey(),
		Data:   &signing.SingleSignatureData{SignMode: signing.SignMode_SIGN_MODE_DIRECT, Signature: []byte("unverified")},
	}))

	raw, err := cfg.TxEncoder()(b.GetTx())
	require.NoError(t, err)

	return raw
}

func startAkash(t *testing.T, sc simnode.Scenario, opts simnode.Options) (*simnode.Node, *clock.Fake) {
	t.Helper()

	fake := clock.NewFake(start)
	if opts.Clock == nil {
		opts.Clock = fake
	}

	n, err := simnode.Start(t.Context(), sc, opts)
	require.NoError(t, err)
	t.Cleanup(n.Close)

	return n, fake
}

func comet(t *testing.T, n *simnode.Node) *rpchttp.HTTP {
	t.Helper()

	c, err := rpchttp.New(n.CometURL, "/websocket")
	require.NoError(t, err)

	return c
}

// registry resolves the auth account types in query answers.
func registry() codectypes.InterfaceRegistry {
	reg := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(reg)
	authtypes.RegisterInterfaces(reg)

	return reg
}

func grpcConn(t *testing.T, n *simnode.Node) *grpc.ClientConn {
	t.Helper()

	cdc := codec.NewProtoCodec(registry())
	conn, err := grpc.NewClient(n.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(cdc.GRPCCodec())))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// pinned is the metadata that pins a gRPC read to h, as the reporter sends it.
func pinned(h int64) metadata.MD {
	return metadata.Pairs(grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(h, 10))
}

func sinkTransfer(t *testing.T, q sinkv1.QueryClient, h int64, id sinkid.ID) sinkv1.TransferState {
	t.Helper()

	ctx := metadata.NewOutgoingContext(t.Context(), pinned(h))
	res, err := q.Transfer(ctx, &sinkv1.QueryTransferRequest{ID: id.Proto()})
	require.NoError(t, err)

	return res.Transfer
}

func latestHeight(t *testing.T, c *rpchttp.HTTP) int64 {
	t.Helper()

	st, err := c.Status(t.Context())
	require.NoError(t, err)

	return st.SyncInfo.LatestBlockHeight
}

func latestTime(t *testing.T, c *rpchttp.HTTP) time.Time {
	t.Helper()

	st, err := c.Status(t.Context())
	require.NoError(t, err)

	return st.SyncInfo.LatestBlockTime
}

func broadcast(t *testing.T, c *rpchttp.HTTP, raw []byte) (code uint32, codespace, log string) {
	t.Helper()

	res, err := c.BroadcastTxSync(t.Context(), raw)
	require.NoError(t, err)
	require.Equal(t, cmttypes.Tx(raw).Hash(), []byte(res.Hash))

	return res.Code, res.Codespace, res.Log
}

func txResult(t *testing.T, c *rpchttp.HTTP, raw []byte) (height int64, code uint32, codespace, log string) {
	t.Helper()

	res, err := c.Tx(t.Context(), cmttypes.Tx(raw).Hash(), false)
	require.NoError(t, err)

	return res.Height, res.TxResult.Code, res.TxResult.Codespace, res.TxResult.Log
}

func TestHeightHeaderEcho(t *testing.T) {
	c := newChain(t)
	n, _ := startAkash(t, c.scenario(t), simnode.Options{})
	q := sinkv1.NewQueryClient(grpcConn(t, n))

	var header metadata.MD
	ctx := metadata.NewOutgoingContext(t.Context(), pinned(7))
	_, err := q.State(ctx, &sinkv1.QueryStateRequest{}, grpc.Header(&header))
	require.NoError(t, err)
	require.Equal(t, []string{"7"}, header.Get(grpctypes.GRPCBlockHeightHeader))

	header = nil
	_, err = q.State(t.Context(), &sinkv1.QueryStateRequest{}, grpc.Header(&header))
	require.NoError(t, err)
	require.Equal(t, []string{"10"}, header.Get(grpctypes.GRPCBlockHeightHeader))

	// A future or pruned height fails, as BaseApp's query context does.
	_, err = q.State(metadata.NewOutgoingContext(t.Context(), pinned(latest+1)), &sinkv1.QueryStateRequest{})
	require.ErrorContains(t, err, "cannot query with height in the future")
	_, err = q.State(metadata.NewOutgoingContext(t.Context(), pinned(4)), &sinkv1.QueryStateRequest{})
	require.ErrorContains(t, err, "failed to load state at height 4")

	rpc := comet(t, n)
	res, err := rpc.ABCIQueryWithOptions(t.Context(), "/akash.sink.v1.Query/Params", nil, rpcclient.ABCIQueryOptions{Height: 8})
	require.NoError(t, err)
	require.Zero(t, res.Response.Code, res.Response.Log)
	require.EqualValues(t, 8, res.Response.Height)
	var params sinkv1.QueryParamsResponse
	require.NoError(t, params.Unmarshal(res.Response.Value))

	// An unpinned query answers at the latest height and echoes it, as BaseApp.Query does.
	res, err = rpc.ABCIQueryWithOptions(t.Context(), "/akash.sink.v1.Query/Params", nil, rpcclient.ABCIQueryOptions{})
	require.NoError(t, err)
	require.Zero(t, res.Response.Code, res.Response.Log)
	require.EqualValues(t, latest, res.Response.Height)
	require.True(t, params.Params.ReportersThreshold.Equal(threshold))
	require.Equal(t, challenge, params.Params.ReporterChallengePeriod)

	// The echo option answers a different height on both transports.
	n.Mutate(func(sc *simnode.Scenario) { sc.Akash.EchoOffset = 1 })
	header = nil
	_, err = q.State(metadata.NewOutgoingContext(t.Context(), pinned(7)), &sinkv1.QueryStateRequest{}, grpc.Header(&header))
	require.NoError(t, err)
	require.Equal(t, []string{"8"}, header.Get(grpctypes.GRPCBlockHeightHeader))
	res, err = rpc.ABCIQueryWithOptions(t.Context(), "/akash.sink.v1.Query/State", nil, rpcclient.ABCIQueryOptions{Height: 7})
	require.NoError(t, err)
	require.EqualValues(t, 8, res.Response.Height)
}

func TestABCIQueryNotFound(t *testing.T) {
	c := newChain(t)
	n, _ := startAkash(t, c.scenario(t), simnode.Options{})

	req, err := (&sinkv1.QueryTransferRequest{ID: sinkID(t, 0x77, 9, 9).Proto()}).Marshal()
	require.NoError(t, err)

	res, err := comet(t, n).ABCIQueryWithOptions(t.Context(), "/akash.sink.v1.Query/Transfer", req, rpcclient.ABCIQueryOptions{Height: latest})
	require.NoError(t, err)
	require.EqualValues(t, 22, res.Response.Code)
	require.Equal(t, "sdk", res.Response.Codespace)
	require.EqualValues(t, latest, res.Response.Height)

	// Unpinned, the error echoes the latest height too.
	res, err = comet(t, n).ABCIQueryWithOptions(t.Context(), "/akash.sink.v1.Query/Transfer", req, rpcclient.ABCIQueryOptions{})
	require.NoError(t, err)
	require.EqualValues(t, 22, res.Response.Code)
	require.EqualValues(t, latest, res.Response.Height)

	// A known transfer answers with its protobuf encoding.
	req, err = (&sinkv1.QueryTransferRequest{ID: c.vote.Proto()}).Marshal()
	require.NoError(t, err)
	res, err = comet(t, n).ABCIQueryWithOptions(t.Context(), "/akash.sink.v1.Query/Transfer", req, rpcclient.ABCIQueryOptions{Height: latest})
	require.NoError(t, err)
	require.Zero(t, res.Response.Code)
	var got sinkv1.QueryTransferResponse
	require.NoError(t, got.Unmarshal(res.Response.Value))
	require.Equal(t, c.vote.Proto(), got.Transfer.ID)
	require.Equal(t, sinkv1.StatusTransferPending, got.Transfer.Status)
}

func TestGRPCNotFound(t *testing.T) {
	c := newChain(t)
	n, _ := startAkash(t, c.scenario(t), simnode.Options{})
	conn := grpcConn(t, n)

	_, err := sinkv1.NewQueryClient(conn).Transfer(t.Context(), &sinkv1.QueryTransferRequest{ID: sinkID(t, 0x77, 9, 9).Proto()})
	require.Equal(t, codes.NotFound, status.Code(err))

	_, err = authtypes.NewQueryClient(conn).Account(t.Context(), &authtypes.QueryAccountRequest{Address: bech(t, bytes.Repeat([]byte{0x99}, 20))})
	require.Equal(t, codes.NotFound, status.Code(err))

	res, err := authtypes.NewQueryClient(conn).Account(t.Context(), &authtypes.QueryAccountRequest{Address: c.reporter})
	require.NoError(t, err)
	var acc sdk.AccountI
	require.NoError(t, registry().UnpackAny(res.Account, &acc))
	require.EqualValues(t, 42, acc.GetAccountNumber())

	info, err := cmtservice.NewServiceClient(conn).GetNodeInfo(t.Context(), &cmtservice.GetNodeInfoRequest{})
	require.NoError(t, err)
	require.Equal(t, akashNodeID, info.DefaultNodeInfo.DefaultNodeID)
	require.Equal(t, akashChain, info.DefaultNodeInfo.Network)
}

func TestSimulateMessageIndex(t *testing.T) {
	c := newChain(t)
	sc := c.scenario(t)
	sc.Akash.SimFailures = []simnode.SimFailure{{ID: c.refundA.String(), Error: "injected failure"}}
	n, _ := startAkash(t, sc, simnode.Options{})
	svc := txtypes.NewServiceClient(grpcConn(t, n))
	timeout := start.Add(latestOffset + time.Minute)

	res, err := svc.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: c.tx(t, timeout, "", c.voteMsg(c.vote, 1), c.voteMsg(c.vote2, 1))})
	require.NoError(t, err)
	require.Positive(t, res.GasInfo.GasUsed)

	msgs := []sdk.Msg{
		c.voteMsg(c.vote, 1),
		c.voteMsg(c.vote2, 1),
		&sinkv1.MsgFinalizeRefund{Executor: c.reporter, ID: c.refundA.Proto()},
	}
	_, err = svc.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: c.tx(t, timeout, "", msgs...)})
	require.ErrorContains(t, err, "message index: 2")
	require.ErrorContains(t, err, "injected failure")

	// The keeper's own failures carry the index too, and simulation changes no state.
	_, err = svc.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: c.tx(t, timeout, "", c.voteMsg(c.vote, 1), c.voteMsg(c.vote2, 5))})
	require.ErrorContains(t, err, "message index: 1")
	require.ErrorContains(t, err, sinkv1.ErrStaleReport.Error())
	require.Empty(t, sinkTransfer(t, sinkv1.NewQueryClient(grpcConn(t, n)), latest, c.vote).Settlement.Votes)

	// A failure outside the messages has no index.
	_, err = svc.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: c.tx(t, start.Add(-time.Hour), "", c.voteMsg(c.vote, 1))})
	require.ErrorContains(t, err, "unordered transaction has a timeout_timestamp that has already passed")
	require.NotContains(t, err.Error(), "message index")
}

func TestCommitBlockIncludesBroadcasts(t *testing.T) {
	c := newChain(t)
	n, _ := startAkash(t, c.scenario(t), simnode.Options{})
	rpc := comet(t, n)

	raw := c.tx(t, start.Add(latestOffset+time.Minute), "", c.voteMsg(c.vote, 1))
	code, _, log := broadcast(t, rpc, raw)
	require.Zero(t, code, log)
	require.Equal(t, [][]byte{raw}, n.Broadcasts())

	_, err := rpc.Tx(t.Context(), cmttypes.Tx(raw).Hash(), false)
	require.ErrorContains(t, err, "not found")

	n.CommitBlock()
	h, code, _, log := txResult(t, rpc, raw)
	require.Zero(t, code, log)
	require.Equal(t, latest+1, h)
	require.Equal(t, latest+1, latestHeight(t, rpc))

	block, err := rpc.Block(t.Context(), &h)
	require.NoError(t, err)
	require.Equal(t, cmttypes.Txs{raw}, block.Block.Txs)
	require.Equal(t, akashChain, block.Block.ChainID)

	info, err := rpc.ABCIInfo(t.Context())
	require.NoError(t, err)
	require.Equal(t, latest+1, info.Response.LastBlockHeight)

	st, err := rpc.Status(t.Context())
	require.NoError(t, err)
	require.True(t, st.TxIndexEnabled())
	require.Equal(t, akashNodeID, string(st.NodeInfo.DefaultNodeID))

	// Without a transaction index, the node reports it and tx fails.
	n.Mutate(func(sc *simnode.Scenario) { sc.Akash.NoTxIndex = true })
	st, err = rpc.Status(t.Context())
	require.NoError(t, err)
	require.False(t, st.TxIndexEnabled())
	_, err = rpc.Tx(t.Context(), cmttypes.Tx(raw).Hash(), false)
	require.ErrorContains(t, err, "transaction indexing is disabled")
}

func TestBroadcastInjection(t *testing.T) {
	c := newChain(t)
	sc := c.scenario(t)
	sc.Akash.BroadcastFaults = []simnode.BroadcastFault{
		{Kind: simnode.FaultTransport},
		{Kind: simnode.FaultLostResponse},
		{Kind: simnode.FaultInCache},
		{Kind: simnode.FaultNonceTaken},
		{Kind: simnode.FaultReject, Code: 5, Codespace: "sdk", Log: "insufficient funds"},
	}
	n, _ := startAkash(t, sc, simnode.Options{})
	rpc := comet(t, n)
	timeout := start.Add(latestOffset + time.Minute)
	txs := make([][]byte, 6)
	for i := range txs {
		txs[i] = c.tx(t, timeout.Add(time.Duration(i)*time.Millisecond), "", c.voteMsg(c.vote, 1))
	}

	// A transport error before admission: nothing is pending.
	_, err := rpc.BroadcastTxSync(t.Context(), txs[0])
	require.Error(t, err)

	// Admitted, then the response is lost.
	_, err = rpc.BroadcastTxSync(t.Context(), txs[1])
	require.Error(t, err)

	_, err = rpc.BroadcastTxSync(t.Context(), txs[2])
	require.ErrorContains(t, err, "tx already exists in cache")

	code, codespace, log := broadcast(t, rpc, txs[3])
	require.EqualValues(t, 18, code)
	require.Equal(t, "sdk", codespace)
	require.Contains(t, log, "failed to add unordered nonce")

	code, codespace, log = broadcast(t, rpc, txs[4])
	require.EqualValues(t, 5, code)
	require.Equal(t, "sdk", codespace)
	require.Equal(t, "insufficient funds", log)

	// Faults are used up: the next attempt is admitted.
	code, _, log = broadcast(t, rpc, txs[5])
	require.Zero(t, code, log)
	require.Equal(t, txs, n.Broadcasts())

	n.CommitBlock()
	for _, i := range []int{0, 2, 3, 4} {
		_, err = rpc.Tx(t.Context(), cmttypes.Tx(txs[i]).Hash(), false)
		require.ErrorContains(t, err, "not found", "tx %d", i)
	}
	_, code, _, log = txResult(t, rpc, txs[1])
	require.Zero(t, code, log)
	// The lost-response copy executed first, so the second vote by the same reporter is a no-op.
	_, code, _, log = txResult(t, rpc, txs[5])
	require.Zero(t, code, log)
	require.Equal(t, map[string]int{voteKey(c.vote): 2}, n.Executions())

	n.Mutate(func(sc *simnode.Scenario) { sc.Akash.UnorderedDisabled = true })
	for i := range 3 {
		code, codespace, log = broadcast(t, rpc, c.tx(t, timeout.Add(time.Second+time.Duration(i)), "", c.voteMsg(c.vote2, 1)))
		require.EqualValues(t, 37, code)
		require.Equal(t, "sdk", codespace)
		require.Contains(t, log, "unordered transactions are not enabled")
	}
}

func TestNamedScenariosStart(t *testing.T) {
	scenarios := simnode.Scenarios()
	const (
		clean = "clean"
		stale = "stale-akash"
		votes = "votes"
	)
	for _, name := range []string{clean, stale, votes} {
		t.Run(name, func(t *testing.T) {
			build, ok := scenarios[name]
			require.True(t, ok)
			sc := build()

			raw, err := json.Marshal(sc)
			require.NoError(t, err)
			var back simnode.Scenario
			require.NoError(t, json.Unmarshal(raw, &back))
			again, err := json.Marshal(back)
			require.NoError(t, err)
			require.JSONEq(t, string(raw), string(again))

			n, err := simnode.Start(t.Context(), sc, simnode.Options{})
			require.NoError(t, err)
			t.Cleanup(n.Close)

			rpc := comet(t, n)
			st, err := rpc.Status(t.Context())
			require.NoError(t, err)
			require.Equal(t, simnode.ChainID, st.NodeInfo.Network)
			age := time.Since(st.SyncInfo.LatestBlockTime)
			if name == stale {
				require.Greater(t, age, 10*time.Minute)
			} else {
				require.Less(t, age, 30*time.Second)
			}

			q := sinkv1.NewQueryClient(grpcConn(t, n))
			reporters, err := q.Reporters(t.Context(), &sinkv1.QueryReportersRequest{})
			require.NoError(t, err)
			require.Len(t, reporters.Reporters, 1)
			require.Contains(t, reporters.Reporters[0].Members, simnode.Reporter)

			want := map[string]sinkv1.Status{clean: sinkv1.StatusTransferPending, stale: sinkv1.StatusTransferPending, votes: sinkv1.StatusVoided}[name]
			page, err := q.Transfers(t.Context(), &sinkv1.QueryTransfersRequest{Status: want, Pagination: &sdkquery.PageRequest{Limit: 500}})
			require.NoError(t, err)
			require.NotEmpty(t, page.Transfers)
			if name == votes {
				require.Len(t, page.Transfers, 30)
				for _, tr := range page.Transfers {
					require.NotNil(t, tr.Settlement.VoidAuthorization)
					require.Empty(t, tr.Settlement.Votes)
				}
			}
		})
	}
}

func TestReplayKeyRule(t *testing.T) {
	c := newChain(t)
	n, fake := startAkash(t, c.scenario(t), simnode.Options{})
	rpc := comet(t, n)
	q := sinkv1.NewQueryClient(grpcConn(t, n))
	timeout := start.Add(latestOffset + time.Minute)

	first := c.tx(t, timeout, "", c.voteMsg(c.vote, 1))
	second := c.tx(t, timeout, "", c.voteMsg(c.vote2, 1))

	code, _, log := broadcast(t, rpc, first)
	require.Zero(t, code, log)
	code, codespace, log := broadcast(t, rpc, second)
	require.EqualValues(t, 18, code)
	require.Equal(t, "sdk", codespace)
	require.Contains(t, log, "failed to add unordered nonce")

	// Another node accepted the second one: it reaches the same block and fails the ante handler there.
	n.Gossip(second)
	n.CommitBlock()
	_, code, _, log = txResult(t, rpc, first)
	require.Zero(t, code, log)
	_, code, codespace, log = txResult(t, rpc, second)
	require.EqualValues(t, 18, code)
	require.Equal(t, "sdk", codespace)
	require.Contains(t, log, "failed to add unordered nonce")
	require.Len(t, sinkTransfer(t, q, latest+1, c.vote).Settlement.Votes, 1)
	require.Empty(t, sinkTransfer(t, q, latest+1, c.vote2).Settlement.Votes)

	// A transaction included with a failure leaves the cache, so its bytes reach CheckTx again; the key is still held.
	code, codespace, log = broadcast(t, rpc, second)
	require.EqualValues(t, 18, code)
	require.Equal(t, "sdk", codespace)
	require.Contains(t, log, "failed to add unordered nonce")

	// Once a block's time passes the timeout, the key is gone and the timeout is refused as expired.
	fake.Advance(2 * time.Minute)
	n.CommitBlock()
	require.True(t, latestTime(t, rpc).After(timeout))
	third := c.tx(t, timeout, "third", c.voteMsg(c.vote2, 1))
	code, _, log = broadcast(t, rpc, third)
	require.EqualValues(t, 18, code)
	require.Contains(t, log, "unordered transaction has a timeout_timestamp that has already passed")

	tooFar := c.tx(t, latestTime(t, rpc).Add(10*time.Minute+time.Second), "", c.voteMsg(c.vote2, 1))
	code, _, log = broadcast(t, rpc, tooFar)
	require.EqualValues(t, 18, code)
	require.Contains(t, log, "unordered tx ttl exceeds 10m0s")

	require.Equal(t, map[string]int{voteKey(c.vote): 1}, n.Executions())

	_, err := rpc.BroadcastTxSync(t.Context(), first)
	require.ErrorContains(t, err, "tx already exists in cache")
}

func TestBlockTimeAvoidsReplayTimeouts(t *testing.T) {
	c := newChain(t)
	n, fake := startAkash(t, c.scenario(t), simnode.Options{})
	rpc := comet(t, n)

	// The clock lands the next block exactly on a held key's timeout: the block moves 1 ns later, so the key is
	// dropped and the pending copy is refused as expired, never executed in the boundary block.
	timeout := start.Add(latestOffset + 30*time.Second)
	held := c.tx(t, timeout, "", c.voteMsg(c.vote, 1))
	code, _, log := broadcast(t, rpc, held)
	require.Zero(t, code, log)
	n.CommitBlock()

	fake.Advance(30 * time.Second)
	n.CommitBlock()
	require.Equal(t, timeout.Add(time.Nanosecond), latestTime(t, rpc))
}

func TestMessageEffectsVersioned(t *testing.T) {
	c := newChain(t)
	n, fake := startAkash(t, c.scenario(t), simnode.Options{})
	rpc := comet(t, n)
	q := sinkv1.NewQueryClient(grpcConn(t, n))
	next := start.Add(latestOffset + time.Minute)
	send := func(msgs ...sdk.Msg) []byte {
		next = next.Add(time.Millisecond)
		raw := c.tx(t, next, "", msgs...)
		code, _, log := broadcast(t, rpc, raw)
		require.Zero(t, code, log)
		n.CommitBlock()

		return raw
	}

	send(c.voteMsg(c.vote, 1))
	h := latest + 1
	require.Empty(t, sinkTransfer(t, q, h-1, c.vote).Settlement.Votes)
	votes := sinkTransfer(t, q, h, c.vote).Settlement.Votes
	require.Len(t, votes, 1)
	require.Equal(t, c.reporter, votes[0].Reporter)
	require.EqualValues(t, 1, votes[0].PolicyVersion)

	raw := send(c.voteMsg(c.vote, 1))
	_, code, _, log := txResult(t, rpc, raw)
	require.Zero(t, code, log)
	require.Len(t, sinkTransfer(t, q, h+1, c.vote).Settlement.Votes, 1)
	require.Equal(t, 2, n.Executions()[voteKey(c.vote)])

	// A stale round fails delivery with the module's code; nothing in the transaction applies.
	raw = send(c.voteMsg(c.vote2, 1), c.voteMsg(c.vote2, 2))
	_, code, codespace, log := txResult(t, rpc, raw)
	require.Equal(t, sinkv1.ErrStaleReport.ABCICode(), code)
	require.Equal(t, sinkv1.ModuleName, codespace)
	require.Contains(t, log, "message index: 1")
	require.Empty(t, sinkTransfer(t, q, h+2, c.vote2).Settlement.Votes)

	send(&sinkv1.MsgVetoVoid{Signer: c.reporter, ID: c.veto.Proto(), Reason: "no status"})
	vetoed := sinkTransfer(t, q, h+3, c.veto).Settlement
	require.Equal(t, sinkv1.HoldVeto, vetoed.Hold)
	require.Equal(t, c.reporter, vetoed.VetoBy)
	require.Equal(t, "no status", vetoed.VetoReason)
	require.EqualValues(t, 1, vetoed.VetoPolicyVersion)
	require.Equal(t, sinkv1.HoldNone, sinkTransfer(t, q, h+2, c.veto).Settlement.Hold)

	// A veto outside refund-pending fails.
	raw = send(&sinkv1.MsgVetoVoid{Signer: c.reporter, ID: c.vote.Proto(), Reason: "x"})
	_, code, codespace, _ = txResult(t, rpc, raw)
	require.Equal(t, sinkv1.ErrNoPendingVoid.ABCICode(), code)
	require.Equal(t, sinkv1.ModuleName, codespace)

	stateAt := func(height int64) *sinkv1.QueryStateResponse {
		res, err := q.State(metadata.NewOutgoingContext(t.Context(), pinned(height)), &sinkv1.QueryStateRequest{})
		require.NoError(t, err)

		return res
	}
	require.Equal(t, c.refundA.Proto(), *stateAt(h + 4).RefundHead)

	// Only the head can be refunded.
	raw = send(&sinkv1.MsgFinalizeRefund{Executor: c.reporter, ID: c.refundB.Proto()})
	_, code, _, _ = txResult(t, rpc, raw)
	require.Equal(t, sinkv1.ErrRefundOutOfOrder.ABCICode(), code)

	send(&sinkv1.MsgFinalizeRefund{Executor: c.reporter, ID: c.refundA.Proto()})
	require.Equal(t, sinkv1.StatusRefunded, sinkTransfer(t, q, h+6, c.refundA).Status)
	require.Equal(t, sinkv1.StatusRefundPending, sinkTransfer(t, q, h+5, c.refundA).Status)
	require.Equal(t, c.refundB.Proto(), *stateAt(h + 6).RefundHead)
	require.Equal(t, c.refundA.Proto(), *stateAt(h + 5).RefundHead)

	// A repeated refund is a code-0 no-op.
	raw = send(&sinkv1.MsgFinalizeRefund{Executor: c.reporter, ID: c.refundA.Proto()})
	_, code, _, log = txResult(t, rpc, raw)
	require.Zero(t, code, log)
	require.Equal(t, 2, n.Executions()[typeRefund+"|"+c.refundA.String()])

	// A governance resolution starts round 2 in its own block, with a new challenge window from that block's time.
	fake.Advance(10 * time.Second)
	evidence := sinkv1.Observation{Slot: 77, TransactionSignature: bytes.Repeat([]byte{7}, 64)}
	n.Resolve(c.veto, sinkv1.StatusRefundPending, evidence)
	resolved := sinkTransfer(t, q, latestHeight(t, rpc), c.veto)
	require.Equal(t, h+8, latestHeight(t, rpc))
	require.Equal(t, sinkv1.StatusRefundPending, resolved.Status)
	s := resolved.Settlement
	require.EqualValues(t, 2, s.Round)
	require.Equal(t, sinkv1.HoldNone, s.Hold)
	require.Empty(t, s.Votes)
	require.Empty(t, s.VetoBy)
	require.Empty(t, s.VetoReason)
	require.Zero(t, s.VetoPolicyVersion)
	end := latestTime(t, rpc).Add(challenge)
	wantEnd := end.Unix()
	if end.Nanosecond() > 0 {
		wantEnd++
	}
	require.Equal(t, wantEnd, s.ChallengeEnd)
	// A confirmed void records governance's observation, taking the slot and signature from the evidence.
	require.Equal(t, &sinkv1.Observation{Outcome: sinkv1.OutcomeVoided, Slot: 77, TransactionSignature: evidence.TransactionSignature},
		s.GovernanceObservation)

	// A veto executes again in the new round, keyed by the round it executed in.
	send(&sinkv1.MsgVetoVoid{Signer: c.reporter, ID: c.veto.Proto(), Reason: "again"})
	require.Equal(t, map[string]int{
		voteKey(c.vote):                         2,
		typeVeto + "|" + c.veto.String() + "|1": 1,
		typeVeto + "|" + c.veto.String() + "|2": 1,
		typeRefund + "|" + c.refundA.String():   2,
	}, n.Executions())

	// A confirmed mint records a minted observation; a restart records none.
	n.Resolve(c.vote2, sinkv1.StatusMinted)
	minted := sinkTransfer(t, q, latestHeight(t, rpc), c.vote2)
	require.Equal(t, sinkv1.StatusMinted, minted.Status)
	require.Equal(t, &sinkv1.Observation{Outcome: sinkv1.OutcomeMinted}, minted.Settlement.GovernanceObservation)
	require.Zero(t, minted.Settlement.ChallengeEnd)

	n.Resolve(c.veto, sinkv1.StatusVoided)
	restarted := sinkTransfer(t, q, latestHeight(t, rpc), c.veto)
	require.Equal(t, sinkv1.StatusVoided, restarted.Status)
	require.EqualValues(t, 3, restarted.Settlement.Round)
	require.Nil(t, restarted.Settlement.GovernanceObservation)
	require.Zero(t, restarted.Settlement.ChallengeEnd)
}

// TestVoteBranches covers each branch of ReportOutcome (x/sink/keeper/settlement.go:88-150) under policy 2.
func TestVoteBranches(t *testing.T) {
	c := newChain(t)
	v2 := c.policy
	v2.Version = 2
	other := c.policy.Members[1]

	vote := func(id sinkid.ID, version, round uint64, outcome sinkv1.Outcome) *sinkv1.MsgReportOutcome {
		m := c.voteMsg(id, round)
		m.PolicyVersion, m.Observation.Outcome = version, outcome

		return m
	}
	ballot := func(reporter string, outcome sinkv1.Outcome, version uint64) sinkv1.Vote {
		return sinkv1.Vote{Reporter: reporter, Observation: sinkv1.Observation{Outcome: outcome}, PolicyVersion: version}
	}
	under := func(id sinkid.ID, st sinkv1.Status, edit func(*sinkv1.SettlementData)) simnode.Transfer {
		tr := c.transfer(id, st, 0)
		tr.Settlement.Policy = v2
		edit(&tr.Settlement)

		return simnode.Transfer{Transfer: tr}
	}

	var (
		refresh      = sinkID(t, 0x21, 3, 1)
		conflictHeld = sinkID(t, 0x22, 3, 2)
		terminal     = sinkID(t, 0x23, 3, 3)
		refundWait   = sinkID(t, 0x24, 3, 4)
		held         = sinkID(t, 0x25, 3, 5)
		voted        = sinkID(t, 0x26, 3, 6)
		conflict     = sinkID(t, 0x27, 3, 7)
	)

	stale := c.transfer(refresh, sinkv1.StatusTransferPending, start.Unix())
	stale.Settlement.Votes = []sinkv1.Vote{ballot(other, sinkv1.OutcomeVoided, 1)}

	sc := c.scenario(t)
	sc.Akash.Policies = append(sc.Akash.Policies, v2)
	sc.Akash.Transfers = []simnode.Transfer{
		{Transfer: stale},
		under(conflictHeld, sinkv1.StatusTransferPending, func(s *sinkv1.SettlementData) { s.Hold = sinkv1.HoldConflict }),
		under(terminal, sinkv1.StatusMinted, func(*sinkv1.SettlementData) {}),
		under(refundWait, sinkv1.StatusRefundPending, func(s *sinkv1.SettlementData) { s.ChallengeEnd = start.Unix() }),
		under(held, sinkv1.StatusTransferPending, func(s *sinkv1.SettlementData) { s.Hold = sinkv1.HoldVeto }),
		under(voted, sinkv1.StatusTransferPending, func(s *sinkv1.SettlementData) {
			s.Votes = []sinkv1.Vote{ballot(c.reporter, sinkv1.OutcomeVoided, 2)}
		}),
		under(conflict, sinkv1.StatusTransferPending, func(s *sinkv1.SettlementData) {
			s.Votes = []sinkv1.Vote{ballot(other, sinkv1.OutcomeMinted, 2)}
		}),
	}
	before := map[string]sinkv1.TransferState{}
	for _, tr := range sc.Akash.Transfers {
		id, err := sinkid.FromProto(tr.Transfer.ID)
		require.NoError(t, err)
		before[id.String()] = tr.Transfer
	}

	n, _ := startAkash(t, sc, simnode.Options{})
	rpc := comet(t, n)
	q := sinkv1.NewQueryClient(grpcConn(t, n))
	next := start.Add(latestOffset + time.Minute)
	send := func(msgs ...sdk.Msg) (code uint32, codespace string) {
		next = next.Add(time.Millisecond)
		raw := c.tx(t, next, "", msgs...)
		code, _, log := broadcast(t, rpc, raw)
		require.Zero(t, code, log)
		n.CommitBlock()
		_, code, codespace, _ = txResult(t, rpc, raw)

		return code, codespace
	}

	// The message's policy must be the current one, and its round the settlement's.
	for _, m := range []*sinkv1.MsgReportOutcome{
		vote(refresh, 1, 1, sinkv1.OutcomeVoided),
		vote(conflict, 2, 2, sinkv1.OutcomeVoided),
	} {
		code, codespace := send(m)
		require.Equal(t, sinkv1.ErrStaleReport.ABCICode(), code)
		require.Equal(t, sinkv1.ModuleName, codespace)
	}
	require.Empty(t, n.Executions())

	ids := []sinkid.ID{refresh, conflictHeld, terminal, refundWait, held, voted, conflict}
	msgs := make([]sdk.Msg, len(ids))
	for i, id := range ids {
		msgs[i] = vote(id, 2, 1, sinkv1.OutcomeVoided)
	}
	code, _ := send(msgs...)
	require.Zero(t, code)
	h := latest + 3

	// A stale settlement policy is refreshed: the current policy, no earlier votes, no challenge end, and the status
	// back to void-requested since a void order exists. Then the vote is recorded.
	got := sinkTransfer(t, q, h, refresh)
	require.Equal(t, sinkv1.StatusVoided, got.Status)
	require.Equal(t, v2.Version, got.Settlement.Policy.Version)
	require.Zero(t, got.Settlement.ChallengeEnd)
	require.Equal(t, []sinkv1.Vote{ballot(c.reporter, sinkv1.OutcomeVoided, 2)}, stripVotes(got.Settlement.Votes))

	// A conflict hold, a terminal, refund-pending or held transfer, and a repeated reporter change nothing.
	for _, id := range []sinkid.ID{conflictHeld, terminal, refundWait, held, voted} {
		want := before[id.String()]
		got := sinkTransfer(t, q, h, id)
		require.Equal(t, want.Status, got.Status, id.String())
		require.Equal(t, want.Settlement.Hold, got.Settlement.Hold, id.String())
		require.Equal(t, stripVotes(want.Settlement.Votes), stripVotes(got.Settlement.Votes), id.String())
	}

	// A contradicting vote is recorded and holds the transfer for governance.
	got = sinkTransfer(t, q, h, conflict)
	require.Equal(t, sinkv1.HoldConflict, got.Settlement.Hold)
	require.Equal(t, []sinkv1.Vote{
		ballot(other, sinkv1.OutcomeMinted, 2),
		ballot(c.reporter, sinkv1.OutcomeVoided, 2),
	}, stripVotes(got.Settlement.Votes))

	// Every one of them executed with code 0, the no-ops included.
	want := map[string]int{}
	for _, id := range ids {
		want[typeVote+"|"+id.String()+"|"+strconv.Itoa(int(sinkv1.OutcomeVoided))+"|1|2"] = 1
	}
	require.Equal(t, want, n.Executions())
}

// stripVotes keeps the fields the branches set, so votes compare equal however their evidence was built.
func stripVotes(votes []sinkv1.Vote) []sinkv1.Vote {
	out := make([]sinkv1.Vote, len(votes))
	for i, v := range votes {
		out[i] = sinkv1.Vote{Reporter: v.Reporter, Observation: sinkv1.Observation{Outcome: v.Observation.Outcome}, PolicyVersion: v.PolicyVersion}
	}

	return out
}

func TestTransfersPagination(t *testing.T) {
	c := newChain(t)
	n, _ := startAkash(t, c.scenario(t), simnode.Options{})
	q := sinkv1.NewQueryClient(grpcConn(t, n))
	ctx := metadata.NewOutgoingContext(t.Context(), pinned(latest))

	ids := func(ts []sinkv1.TransferState) []string {
		out := make([]string, len(ts))
		for i, tr := range ts {
			id, err := sinkid.FromProto(tr.ID)
			require.NoError(t, err)
			out[i] = id.String()
		}

		return out
	}

	// A status bucket walks record-key order; next_key resumes it, and the resumed page ignores a changed filter.
	page, err := q.Transfers(ctx, &sinkv1.QueryTransfersRequest{Status: sinkv1.StatusRefundPending, Pagination: &sdkquery.PageRequest{Limit: 2}})
	require.NoError(t, err)
	require.Equal(t, []string{c.veto.String(), c.refundA.String()}, ids(page.Transfers))
	require.NotEmpty(t, page.Pagination.NextKey)
	page, err = q.Transfers(ctx, &sinkv1.QueryTransfersRequest{Status: sinkv1.StatusMinted, Pagination: &sdkquery.PageRequest{Key: page.Pagination.NextKey, Limit: 2}})
	require.NoError(t, err)
	require.Equal(t, []string{c.refundB.String()}, ids(page.Transfers))
	require.Empty(t, page.Pagination.NextKey)

	// Without filters, the chronological history: by height, then sequence.
	page, err = q.Transfers(ctx, &sinkv1.QueryTransfersRequest{Pagination: &sdkquery.PageRequest{Limit: 100}})
	require.NoError(t, err)
	require.Equal(t, []string{c.held.String(), c.refundB.String(), c.vote.String(), c.vote2.String(), c.veto.String(), c.refundA.String()}, ids(page.Transfers))

	page, err = q.Transfers(ctx, &sinkv1.QueryTransfersRequest{Owner: c.veto.Owner})
	require.NoError(t, err)
	require.Equal(t, []string{c.veto.String()}, ids(page.Transfers))

	_, err = q.Transfers(ctx, &sinkv1.QueryTransfersRequest{Pagination: &sdkquery.PageRequest{Offset: 1}})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	disputes, err := q.Disputes(ctx, &sinkv1.QueryDisputesRequest{})
	require.NoError(t, err)
	require.Equal(t, []string{c.held.String()}, ids(disputes.Transfers))

	// The same pages over abci_query, as the TS client reads them.
	req, err := (&sinkv1.QueryTransfersRequest{Status: sinkv1.StatusTransferPending, Pagination: &sdkquery.PageRequest{Limit: 500}}).Marshal()
	require.NoError(t, err)
	res, err := comet(t, n).ABCIQueryWithOptions(t.Context(), "/akash.sink.v1.Query/Transfers", cmtbytes.HexBytes(req), rpcclient.ABCIQueryOptions{Height: latest})
	require.NoError(t, err)
	require.Zero(t, res.Response.Code, res.Response.Log)
	var got sinkv1.QueryTransfersResponse
	require.NoError(t, got.Unmarshal(res.Response.Value))
	require.Equal(t, []string{c.vote.String(), c.vote2.String(), c.held.String()}, ids(got.Transfers))
	require.NotNil(t, got.Pagination)

	reporters, err := q.Reporters(ctx, &sinkv1.QueryReportersRequest{Version: math.MaxUint64})
	require.NoError(t, err)
	require.Len(t, reporters.Reporters, 1)
	_, err = q.Reporters(ctx, &sinkv1.QueryReportersRequest{Version: 9})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestLagAndAlign(t *testing.T) {
	c := newChain(t)
	n, _ := startAkash(t, c.scenario(t), simnode.Options{})
	rpc := comet(t, n)

	n.SetLag(2)
	require.Equal(t, latest-2, latestHeight(t, rpc))
	info, err := rpc.ABCIInfo(t.Context())
	require.NoError(t, err)
	require.Equal(t, latest-2, info.Response.LastBlockHeight)
	block, err := rpc.Block(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, latest-2, block.Block.Height)
	var header metadata.MD
	_, err = sinkv1.NewQueryClient(grpcConn(t, n)).State(t.Context(), &sinkv1.QueryStateRequest{}, grpc.Header(&header))
	require.NoError(t, err)
	require.Equal(t, []string{"8"}, header.Get(grpctypes.GRPCBlockHeightHeader))
	n.SetLag(0)
	require.Equal(t, latest, latestHeight(t, rpc))

	const window = 2 * time.Minute
	for _, into := range []time.Duration{10 * time.Second, 65 * time.Second, 100 * time.Second, 119 * time.Second} {
		now := time.Unix(1_794_480_000, 0).Truncate(window).Add(into)
		fake := clock.NewFake(now)
		n, _ := startAkash(t, c.scenario(t), simnode.Options{Clock: fake, AlignWindow: true})
		rpc := comet(t, n)

		bt := latestTime(t, rpc)
		require.Less(t, time.Duration(bt.UnixNano()%int64(window)), time.Minute, "into %s", into)
		behind := now.Sub(bt)
		require.GreaterOrEqual(t, behind, time.Duration(0), "into %s", into)
		require.LessOrEqual(t, behind, 70*time.Second, "into %s", into)

		// Block times then advance with the clock, keeping that offset.
		fake.Advance(5 * time.Second)
		n.CommitBlock()
		require.Equal(t, behind, fake.Now().Sub(latestTime(t, rpc)), "into %s", into)
	}
}

func TestBlockIntervalAndLatency(t *testing.T) {
	c := newChain(t)
	n, fake := startAkash(t, c.scenario(t), simnode.Options{BlockInterval: 6 * time.Second, AkashLatency: 20 * time.Millisecond})
	rpc := comet(t, n)

	began := time.Now()
	require.Equal(t, latest, latestHeight(t, rpc))
	require.GreaterOrEqual(t, time.Since(began), 20*time.Millisecond)

	began = time.Now()
	_, err := sinkv1.NewQueryClient(grpcConn(t, n)).Params(t.Context(), &sinkv1.QueryParamsRequest{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, time.Since(began), 20*time.Millisecond)

	// Blocks follow the clock: none before the interval passes, then one per interval.
	fake.Advance(5 * time.Second)
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, latest, latestHeight(t, rpc))
	require.Eventually(t, func() bool {
		if latestHeight(t, rpc) >= latest+3 {
			return true
		}
		fake.Advance(time.Second)

		return false
	}, 5*time.Second, 5*time.Millisecond)
	require.Positive(t, n.Calls("status"))

	// A halted chain produces no more blocks.
	n.Halt()
	h := latestHeight(t, rpc)
	fake.Advance(time.Minute)
	n.CommitBlock()
	require.Equal(t, h, latestHeight(t, rpc))
}
