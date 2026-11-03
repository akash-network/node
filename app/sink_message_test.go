package app_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtestdata "github.com/CosmWasm/wasmd/x/wasm/keeper/testdata"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

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
	contract, _, err := wasm.Instantiate(ctx, codeID, h.sender, h.sender, init, "sink bank bypass fixture", sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 10)))
	require.NoError(t, err)
	before := h.a.Keepers.Cosmos.Bank.GetAllBalances(ctx, contract)
	msg := &wasmtypes.MsgExecuteContract{Sender: h.sender.String(), Contract: contract.String(), Msg: []byte(`{"release":{}}`)}
	res = h.deliver(t, "", msg)
	require.NotZero(t, res.Code, res.Log)

	// Contract bank messages go through x/bank, which blocks the sink address.
	require.Contains(t, res.Log, "is not allowed to receive funds")
	require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), contract))
	require.Equal(t, uint64(1), h.totals().PendingCount)
	require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
}

func TestSinkMessageValidationAndSigner(t *testing.T) {
	h := newSinkHarness(t, nil)
	for _, change := range []func(*types.MsgSink){
		func(m *types.MsgSink) { m.Dest = "bad" }, func(m *types.MsgSink) { m.Dest = "11111111111111111111111111111111" },
		func(m *types.MsgSink) { m.DestType = types.DestTypeInvalid }, func(m *types.MsgSink) { m.DestType = types.DestTypeSolana + 1 },
		func(m *types.MsgSink) { m.Amount.Denom = "other" }, func(m *types.MsgSink) { m.Amount.Amount = sdkmath.Int{} }, func(m *types.MsgSink) { m.Amount = coin(0) }, func(m *types.MsgSink) { m.Amount.Amount = sdkmath.NewInt(-1) },
		func(m *types.MsgSink) { m.Sender = h.bystander.String() },
	} {
		msg := h.deposit(10)
		change(msg)
		res := h.deliver(t, "", msg)
		require.NotZero(t, res.Code)
		require.Zero(t, h.totals().PendingCount)
	}

	_, err := h.a.Keepers.Akash.Sink.Deposit(h.ctx(), nil)
	require.Error(t, err)
	_, err = h.a.Keepers.Akash.Sink.Deposit(h.ctx(), h.deposit(10))
	require.ErrorIs(t, err, types.ErrDepositNotSole)
}

func TestSinkLegacyAminoSignature(t *testing.T) {
	h := newSinkHarness(t, nil)
	res := h.deliverBytes(t, h.txFromMode(t, h.key, signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, "", h.deposit(10)))
	require.Zero(t, res.Code, res.Log)
	require.Contains(t, string(h.signBytes), `"type":"sink/MsgSink"`)
	require.Equal(t, uint64(1), h.totals().PendingCount)
}

func TestSinkDepositBankFailureAndRedirectRollback(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprint(redirect), func(t *testing.T) {
			h := newSinkHarness(t, nil)
			h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
				if to.Equals(sinkkeeper.Address()) {
					if redirect {
						return h.bystander, nil
					}

					return nil, fmt.Errorf("bank refused")
				}

				return to, nil
			})

			before := h.totals()
			other := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.bystander)
			res := h.deliver(t, "", h.deposit(10))
			require.NotZero(t, res.Code)
			require.Equal(t, before, h.totals())
			require.Equal(t, other, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.bystander))
			require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
		})
	}
}

func TestSinkQueryStatusAndCanonicalIDs(t *testing.T) {
	h := reportingHarness(t)
	q := h.a.Keepers.Akash.Sink.NewQuerier()
	_, err := q.Transfer(h.ctx(), nil)
	require.Error(t, err)

	for _, id := range []types.ID{{}, {Owner: h.sender.String(), Height: 1}, {Owner: h.sender.String(), Height: -1, Sequence: 1}} {
		_, err = q.Transfer(h.ctx(), &types.QueryTransferRequest{ID: id})
		require.Error(t, err)
	}

	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	got, err := q.Transfer(h.ctx(), &types.QueryTransferRequest{ID: h.id(1)})
	require.NoError(t, err)
	require.Equal(t, types.StatusTransferPending, got.Transfer.Settlement.Status)
	h.quorum(t, 1, types.OutcomeVoided)
	h.now = *h.settlement(t, 1).ChallengeEnd
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	got, err = q.Transfer(h.ctx(), &types.QueryTransferRequest{ID: h.id(1)})
	require.NoError(t, err)
	require.Equal(t, types.StatusRefunded, got.Transfer.Settlement.Status)
}

func TestSinkQueryLegacyWireRecord(t *testing.T) {
	h := newSinkHarness(t, nil)
	ctx, _ := h.ctx().CacheContext()
	ctx = ctx.WithBlockHeight(2)
	ctx.KVStore(h.a.GetKey(types.StoreKey)).Set([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 42}, bytes.Repeat([]byte{1}, 77))
	_, err := h.a.Keepers.Akash.Sink.NewQuerier().Transfer(ctx, &types.QueryTransferRequest{ID: types.ID{Owner: h.sender.String(), Height: 1, Sequence: 42}})
	require.ErrorIs(t, err, types.ErrTransferNotFound)
	clearSinkStore(t, h, ctx)
	ctx.KVStore(h.a.GetKey(types.StoreKey)).Set([]byte{2, 0}, []byte{1})
	require.Error(t, h.a.Keepers.Akash.Sink.InitGenesis(ctx, types.DefaultGenesis()))
}

func TestSinkRequestBindingAndNativeCacheRollback(t *testing.T) {
	h := newSinkHarness(t, nil)
	res := h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log)
	k := h.a.Keepers.Akash.Sink
	for _, tc := range []struct{ signed, delivered func(*types.MsgSink) }{
		{delivered: func(m *types.MsgSink) { m.Sender = h.bystander.String() }},
		{delivered: func(m *types.MsgSink) { m.Dest = types.SolanaAddress{1}.String() }},
		{delivered: func(m *types.MsgSink) { require.NoError(t, m.Amount.Amount.Unmarshal([]byte("20"))) }},
		{signed: func(m *types.MsgSink) { m.DestType = types.DestTypeInvalid }, delivered: func(m *types.MsgSink) { m.DestType = types.DestTypeSolana }},
	} {
		msg := h.deposit(10)
		if tc.signed != nil {
			tc.signed(msg)
		}

		b := h.a.TxConfig().NewTxBuilder()
		require.NoError(t, b.SetMsgs(msg))
		_, err := sinkkeeper.NewDepositDecorator().AnteHandle(h.ctx(), b.GetTx(), false, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
			tc.delivered(msg)
			_, err := k.Deposit(ctx, msg)
			return ctx, err
		})
		require.ErrorIs(t, err, types.ErrDepositNotSole)
	}

	redirect := true
	h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
		if redirect && from.Equals(h.sender) && to.Equals(sinkkeeper.Address()) {
			return h.bystander, nil
		}

		return to, nil
	})

	before := h.totals()
	balance := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender)
	ctx := h.ctx().WithBlockHeight(h.height + 1).WithEventManager(sdk.NewEventManager())
	msg := h.deposit(10)
	b := h.a.TxConfig().NewTxBuilder()
	require.NoError(t, b.SetMsgs(msg))
	var id types.ID
	call := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		var err error
		id, err = k.Deposit(ctx, msg)
		return ctx, err
	}
	_, err := sinkkeeper.NewDepositDecorator().AnteHandle(ctx, b.GetTx(), false, call)
	require.Error(t, err)
	require.Empty(t, ctx.EventManager().Events())
	require.Equal(t, before, h.totals())
	require.Equal(t, balance, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender))
	redirect = false
	_, err = sinkkeeper.NewDepositDecorator().AnteHandle(ctx, b.GetTx(), false, call)
	require.NoError(t, err)
	require.Equal(t, uint64(1), id.Sequence)
}
