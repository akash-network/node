package app_test

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtestdata "github.com/CosmWasm/wasmd/x/wasm/keeper/testdata"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/sink/v1"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

func TestSinkMessageValidationAndSigner(t *testing.T) {
	h := newSinkHarness(t, nil)
	k := h.a.Keepers.Akash.Sink
	overflow, ok := sdkmath.NewIntFromString("18446744073709551616")
	require.True(t, ok)
	for _, tc := range []struct {
		name   string
		change func(*types.MsgSink)
	}{
		{"zero destination type", func(m *types.MsgSink) { m.DestType = 0 }},
		{"unknown destination type", func(m *types.MsgSink) { m.DestType = 2 }},
		{"maximum destination type", func(m *types.MsgSink) { m.DestType = math.MaxUint32 }},
		{"bad destination", func(m *types.MsgSink) { m.Dest = "not a Solana address" }},
		{"zero destination", func(m *types.MsgSink) { m.Dest = "11111111111111111111111111111111" }},
		{"wrong denomination", func(m *types.MsgSink) { m.Amount.Denom = "other" }},
		{"nil amount", func(m *types.MsgSink) { m.Amount.Amount = sdkmath.Int{} }},
		{"zero amount", func(m *types.MsgSink) { m.Amount.Amount = sdkmath.ZeroInt() }},
		{"negative amount", func(m *types.MsgSink) { m.Amount.Amount = sdkmath.NewInt(-1) }},
		{"overflow amount", func(m *types.MsgSink) { m.Amount.Amount = overflow }},
		{"mismatched signer", func(m *types.MsgSink) { m.Sender = h.source.String() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := h.deposit(10)
			tc.change(msg)
			before := k.GetTotals(h.ctx())
			balance := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender)
			res := h.deliver(t, "", msg)
			require.NotZero(t, res.Code, res.Log)
			require.Equal(t, before, k.GetTotals(h.ctx()))
			require.NoError(t, k.CheckBalance(h.ctx()))
			// Stateless MsgSink.ValidateBasic rejects these before any fee is taken.
			require.Equal(t, balance, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender))
		})
	}
	_, err := k.Sink(h.ctx(), nil)
	require.ErrorContains(t, err, "nil sink request")
	_, err = k.Sink(h.ctx(), h.deposit(10))
	require.ErrorContains(t, err, "sole top-level MsgSink")
	// Memos carry no destination semantics; the typed message is authoritative.
	res := h.deliver(t, "optional human memo", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
}

func TestSinkLegacyAminoSignature(t *testing.T) {
	h := newSinkHarness(t, nil)
	msg := h.deposit(10)
	bz := h.txFromMode(t, h.key, signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, "", msg)
	// Wallets sign this Amino JSON; the message type name must stay stable.
	require.Contains(t, string(h.signBytes), `"type":"sink/MsgSink"`)
	res := h.deliverBytes(t, bz)
	require.Zero(t, res.Code, res.Log)
	var data sdk.TxMsgData
	require.NoError(t, data.Unmarshal(res.Data))
	require.Len(t, data.MsgResponses, 1)
	var response types.MsgSinkResponse
	require.NoError(t, response.Unmarshal(data.MsgResponses[0].Value))
	require.Equal(t, "1", response.Id)
	require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
}

func TestSinkDepositBankFailureAndRedirectRollback(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprintf("redirect_%t", redirect), func(t *testing.T) {
			h := newSinkHarness(t, nil)
			k := h.a.Keepers.Akash.Sink
			h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
				if from.Equals(h.sender) && to.Equals(sinkkeeper.Address()) {
					if redirect {
						return h.source, nil
					}
					return nil, fmt.Errorf("injected bank rejection")
				}
				return to, nil
			})
			before := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.source)
			res := h.deliver(t, "", h.deposit(10))
			require.NotZero(t, res.Code, res.Log)
			if redirect {
				require.Contains(t, res.Log, "bank balance does not match")
			} else {
				require.Contains(t, res.Log, "injected bank rejection")
			}
			require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.source))
			require.Zero(t, k.GetTotals(h.ctx()).LastTransferId)
			require.Zero(t, k.GetAccountUsage(h.ctx(), h.sender).DailyCount)
			require.NoError(t, k.CheckBalance(h.ctx()))
		})
	}
}

func TestSinkQueryStatusAndCanonicalIDs(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	q := sinkkeeper.QueryServer{Keeper: k}
	for _, id := range []string{"", "0", "01", "+1", "-1", " 1", "1 ", "1.0", "1e0", "18446744073709551616"} {
		_, err := q.Sink(h.ctx(), &types.QuerySink{Id: id})
		require.Equal(t, codes.InvalidArgument, status.Code(err), id)
	}
	_, err := q.Sink(h.ctx(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	for _, id := range []string{"1", "18446744073709551615"} {
		_, err = q.Sink(h.ctx(), &types.QuerySink{Id: id})
		require.Equal(t, codes.NotFound, status.Code(err))
	}
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	pending, err := q.Sink(h.ctx(), &types.QuerySink{Id: "1"})
	require.NoError(t, err)
	require.Equal(t, types.QuerySinkResponse_transfer_pending, pending.Status)
	record := bytes.Clone(pending.Transfer.Record)
	require.NoError(t, k.RequestVoid(h.ctx(), authtypes.NewModuleAddress(govtypes.ModuleName).String(), 1))
	ordered, err := q.Sink(h.ctx(), &types.QuerySink{Id: "1"})
	require.NoError(t, err)
	require.Equal(t, types.QuerySinkResponse_transfer_pending, ordered.Status)
	require.True(t, ordered.Transfer.VoidOrdered)
	h.quorum(t, 1, types.Outcome_OUTCOME_VOIDED)
	voided, err := q.Sink(h.ctx(), &types.QuerySink{Id: "1"})
	require.NoError(t, err)
	require.Equal(t, types.QuerySinkResponse_void, voided.Status)
	require.Equal(t, types.Status_STATUS_VOIDED, voided.Transfer.Settlement.Status)
	require.Positive(t, voided.Transfer.Settlement.ChallengeEnd)
	// Holds remain independently visible; public void does not promise a refund.
	veto := &types.MsgVetoVoid{Signer: sdk.AccAddress(h.reporters[0].PubKey().Address()).String(), Id: 1, Reason: "test review"}
	res = h.deliverBytes(t, h.txFrom(t, h.reporters[0], "", veto))
	require.Zero(t, res.Code, res.Log)
	held, err := q.Sink(h.ctx(), &types.QuerySink{Id: "1"})
	require.NoError(t, err)
	require.Equal(t, types.QuerySinkResponse_void, held.Status)
	require.Equal(t, types.Hold_HOLD_VETO, held.Transfer.Settlement.Hold)
	require.NoError(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_VOID))
	h.now = h.now.Add(20 * time.Second)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	refunded, err := q.Sink(h.ctx(), &types.QuerySink{Id: "1"})
	require.NoError(t, err)
	require.Equal(t, types.QuerySinkResponse_void, refunded.Status)
	require.Equal(t, types.Status_STATUS_REFUNDED, refunded.Transfer.Settlement.Status)
	require.Equal(t, record, refunded.Transfer.Record)
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 2, types.Outcome_OUTCOME_MINTED)
	completed, err := q.Sink(h.ctx(), &types.QuerySink{Id: "2"})
	require.NoError(t, err)
	require.Equal(t, types.QuerySinkResponse_completed, completed.Status)
	require.Equal(t, types.Status_STATUS_MINTED, completed.Transfer.Settlement.Status)
}

func TestSinkQueryLegacyWireRecord(t *testing.T) {
	h := newSinkHarness(t, nil)
	ctx, _ := h.ctx().CacheContext()
	// Historical bank deposits used precisely this wire value and no message-
	// response state. QuerySink must read them without migration or rewriting.
	legacy := types.Transfer{ID: 42, Sender: [20]byte(h.sender), Recipient: h.recipient, Amount: 10, Height: 1}
	record, err := legacy.Marshal()
	require.NoError(t, err)
	ctx.KVStore(h.a.GetKey(types.StoreKey)).Set(types.TransferKey(42), record)
	q := sinkkeeper.QueryServer{Keeper: h.a.Keepers.Akash.Sink}
	got, err := q.Sink(ctx, &types.QuerySink{Id: "42"})
	require.NoError(t, err)
	require.Equal(t, types.QuerySinkResponse_transfer_pending, got.Status)
	require.Equal(t, record, got.Transfer.Record)
	require.Equal(t, uint64(1), got.Transfer.Settlement.Round)
}

func TestSinkContractBankDepositRejected(t *testing.T) {
	h := newSinkHarness(t, nil)
	// Initialize a live block/account before adding a test contract through the
	// native governance setup path. Its execution is a real signed transaction.
	res := h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log)
	ctx := h.ctx()
	wasm := wasmkeeper.NewGovPermissionKeeper(h.a.Keepers.Cosmos.Wasm)
	codeID, _, err := wasm.Create(ctx, h.sender, wasmtestdata.HackatomContractWasm(), &wasmtypes.AllowEverybody)
	require.NoError(t, err)
	init := []byte(fmt.Sprintf(`{"verifier":%q,"beneficiary":%q}`, h.sender.String(), sinkkeeper.Address().String()))
	contract, _, err := wasm.Instantiate(ctx, codeID, h.sender, h.sender, init, "sink bank bypass fixture", sdk.NewCoins(sdk.NewInt64Coin("uakt", 10)))
	require.NoError(t, err)
	before := h.a.Keepers.Cosmos.Bank.GetAllBalances(ctx, contract)
	msg := &wasmtypes.MsgExecuteContract{Sender: h.sender.String(), Contract: contract.String(), Msg: []byte(`{"release":{}}`)}
	res = h.deliver(t, "", msg)
	require.NotZero(t, res.Code, res.Log)
	// Contract bank messages go through x/bank, which blocks the sink address.
	require.Contains(t, res.Log, "is not allowed to receive funds")
	require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), contract))
	require.Equal(t, uint64(1), h.a.Keepers.Akash.Sink.GetTotals(h.ctx()).LastTransferId)
	require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
}

func TestSinkRequestBindingAndNativeCacheRollback(t *testing.T) {
	h := newSinkHarness(t, nil)
	res := h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log) // establishes signer key and a positive height
	k := h.a.Keepers.Akash.Sink
	for _, tc := range []struct {
		name   string
		change func(*types.MsgSink)
	}{
		{"sender", func(m *types.MsgSink) { m.Sender = h.source.String() }},
		{"destination type", func(m *types.MsgSink) { m.DestType = 2 }},
		{"destination", func(m *types.MsgSink) { m.Dest = "another destination" }},
		{"amount", func(m *types.MsgSink) { require.NoError(t, m.Amount.Amount.Unmarshal([]byte("20"))) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := h.deposit(10)
			builder := h.a.TxConfig().NewTxBuilder()
			require.NoError(t, builder.SetMsgs(msg))
			_, err := sinkkeeper.NewDepositDecorator().AnteHandle(h.ctx(), builder.GetTx(), false,
				func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
					tc.change(msg)
					_, err := k.Sink(ctx, msg)
					return ctx, err
				})
			require.ErrorContains(t, err, "sole top-level MsgSink")
		})
	}
	// Exercise keeper atomicity outside BaseApp's cache: a later restriction
	// redirects the transfer. The local cache must undo even that bank write.
	h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
		if from.Equals(h.sender) && to.Equals(sinkkeeper.Address()) {
			return h.source, nil
		}
		return to, nil
	})
	beforeSender := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender)
	beforeSource := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.source)
	beforeTotals := k.GetTotals(h.ctx())
	msg := h.deposit(10)
	builder := h.a.TxConfig().NewTxBuilder()
	require.NoError(t, builder.SetMsgs(msg))
	_, err := sinkkeeper.NewDepositDecorator().AnteHandle(h.ctx(), builder.GetTx(), false,
		func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
			_, err := k.Sink(ctx, msg)
			return ctx, err
		})
	require.ErrorContains(t, err, "bank balance does not match")
	require.Equal(t, beforeSender, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender))
	require.Equal(t, beforeSource, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.source))
	require.Equal(t, beforeTotals, k.GetTotals(h.ctx()))
	require.NoError(t, k.CheckBalance(h.ctx()))
}
