package sink_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtestdata "github.com/CosmWasm/wasmd/x/wasm/keeper/testdata"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	wasmvmtypes "github.com/CosmWasm/wasmvm/v3/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/authz"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	sinkhandler "pkg.akt.dev/node/v2/x/sink/handler"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

func TestSinkContractBankDepositRejected(t *testing.T) {
	h := setupTestSuite(t, nil)

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
	before := captureSinkBankState(t, h, ctx, contract)
	msg := &wasmtypes.MsgExecuteContract{
		Sender:   h.sender.String(),
		Contract: contract.String(),
		Msg:      []byte(`{"release":{}}`),
		Funds:    sdk.NewCoins(coin(3)),
	}

	res = h.deliver(t, "", msg)
	require.NotZero(t, res.Code, res.Log)

	require.Contains(t, res.Log, "is not allowed to receive funds")
	before.balances[h.sender.String()] = before.balances[h.sender.String()].Sub(coin(10))
	require.Equal(t, before, captureSinkBankState(t, h, h.ctx(), contract))
	require.Empty(t, sinkEvents(t, res.Events))
	require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
	t.Logf("contract bank send rejected code=%d log=%s; attached funds and contract balance rolled back", res.Code, res.Log)
}

func TestSinkContractDispatchRejected(t *testing.T) {
	h := setupTestSuite(t, nil)
	res := h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log)
	wasm := wasmkeeper.NewGovPermissionKeeper(h.a.Keepers.Cosmos.Wasm)
	codeID, _, err := wasm.Create(h.ctx(), h.sender, wasmtestdata.ReflectContractWasm(), &wasmtypes.AllowEverybody)
	require.NoError(t, err)
	contract, _, err := wasm.Instantiate(h.ctx(), codeID, h.sender, h.sender, []byte(`{}`), "sink dispatch fixture", sdk.NewCoins(coin(10)))
	require.NoError(t, err)
	deposit := h.deposit(1)
	deposit.Sender = contract.String()
	depositBytes, err := deposit.Marshal()
	require.NoError(t, err)
	exec := authz.NewMsgExec(contract, []sdk.Msg{deposit})
	execBytes, err := exec.Marshal()
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		msg  wasmvmtypes.CosmosMsg
		want string
	}{
		{
			name: "bank send",
			msg: wasmvmtypes.CosmosMsg{
				Bank: &wasmvmtypes.BankMsg{
					Send: &wasmvmtypes.SendMsg{
						ToAddress: sinkkeeper.Address().String(),
						Amount: []wasmvmtypes.Coin{
							{
								Denom:  sdkutil.DenomUakt,
								Amount: "1",
							},
						},
					},
				},
			},
			want: "is not allowed to receive funds",
		},
		{
			name: "wasm execute with funds",
			msg: wasmvmtypes.CosmosMsg{
				Wasm: &wasmvmtypes.WasmMsg{
					Execute: &wasmvmtypes.ExecuteMsg{
						ContractAddr: sinkkeeper.Address().String(),
						Msg:          []byte(`{}`),
						Funds: []wasmvmtypes.Coin{
							{
								Denom:  sdkutil.DenomUakt,
								Amount: "1",
							},
						},
					},
				},
			},
			want: "no such contract",
		},
		{
			name: "protobuf sink",
			msg: wasmvmtypes.CosmosMsg{
				Any: &wasmvmtypes.AnyMsg{
					TypeURL: sdk.MsgTypeURL(deposit),
					Value:   depositBytes,
				},
			},
			want: "Any message type " + sdk.MsgTypeURL(deposit) + " not allowed",
		},
		{
			name: "protobuf authz sink",
			msg: wasmvmtypes.CosmosMsg{
				Any: &wasmvmtypes.AnyMsg{
					TypeURL: sdk.MsgTypeURL(&exec),
					Value:   execBytes,
				},
			},
			want: "Any message type " + sdk.MsgTypeURL(&exec) + " not allowed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(wasmtestdata.ReflectHandleMsg{
				Reflect: &wasmtestdata.ReflectPayload{
					Msgs: []wasmvmtypes.CosmosMsg{tc.msg},
				},
			})
			require.NoError(t, err)
			before := captureSinkBankState(t, h, h.ctx(), contract)
			contractState := storeEntries(t, h.ctx().KVStore(h.a.GetKey(wasmtypes.StoreKey)), wasmtypes.GetContractStorePrefix(contract))
			res := h.deliver(t, "", &wasmtypes.MsgExecuteContract{
				Sender:   h.sender.String(),
				Contract: contract.String(),
				Msg:      body,
				Funds:    sdk.NewCoins(coin(3)),
			})

			require.NotZero(t, res.Code, res.Log)
			require.Contains(t, res.Log, tc.want)
			before.balances[h.sender.String()] = before.balances[h.sender.String()].Sub(coin(10))
			require.Equal(t, before, captureSinkBankState(t, h, h.ctx(), contract))
			require.Equal(t, contractState, storeEntries(t, h.ctx().KVStore(h.a.GetKey(wasmtypes.StoreKey)), wasmtypes.GetContractStorePrefix(contract)))
			require.Empty(t, sinkEvents(t, res.Events))
			require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
			t.Logf("contract dispatch rejected code=%d log=%s; contract and Sink state and transfer balances retained", res.Code, res.Log)
		})
	}

	before := captureSinkBankState(t, h, h.ctx(), contract)
	res = h.deliver(t, "", &wasmtypes.MsgExecuteContract{
		Sender:   h.sender.String(),
		Contract: sinkkeeper.Address().String(),
		Msg:      []byte(`{}`),
		Funds:    sdk.NewCoins(coin(3)),
	})

	require.NotZero(t, res.Code, res.Log)
	require.Contains(t, res.Log, "no such contract")
	before.balances[h.sender.String()] = before.balances[h.sender.String()].Sub(coin(10))
	require.Equal(t, before, captureSinkBankState(t, h, h.ctx(), contract))
	t.Logf("direct wasm attached funds rejected code=%d log=%s; Sink is not a contract", res.Code, res.Log)
}

func TestSinkMessageValidationAndSigner(t *testing.T) {
	h := setupTestSuite(t, nil)
	for _, change := range []func(*types.MsgSink){
		func(m *types.MsgSink) { m.Dest = "bad" },
		func(m *types.MsgSink) { m.Dest = "11111111111111111111111111111111" },
		func(m *types.MsgSink) { m.DestType = types.DestTypeInvalid },
		func(m *types.MsgSink) { m.DestType = types.DestTypeSolana + 1 },
		func(m *types.MsgSink) { m.Amount.Denom = "other" },
		func(m *types.MsgSink) { m.Amount.Amount = sdkmath.Int{} },
		func(m *types.MsgSink) { m.Amount = coin(0) },
		func(m *types.MsgSink) { m.Amount.Amount = sdkmath.NewInt(-1) },
		func(m *types.MsgSink) { m.Sender = h.bystander.String() },
	} {
		msg := h.deposit(10)
		change(msg)
		res := h.deliver(t, "", msg)
		require.NotZero(t, res.Code)
		require.Zero(t, h.totals().PendingCount)
	}

	_, err := sinkhandler.NewServer(h.a.Keepers.Akash.Sink).Sink(h.ctx(), nil)
	require.Error(t, err)
	_, err = h.a.Keepers.Akash.Sink.Deposit(h.ctx(), h.deposit(10))
	require.ErrorIs(t, err, types.ErrDepositNotSole)
}

func TestSinkLegacyAminoSignature(t *testing.T) {
	h := setupTestSuite(t, nil)
	res := h.deliverBytes(t, h.txFromMode(t, h.key, signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, "", h.deposit(10)))
	require.Zero(t, res.Code, res.Log)
	require.Contains(t, string(h.signBytes), `"type":"sink/MsgSink"`)
	require.Equal(t, uint64(1), h.totals().PendingCount)
}

func TestSinkDepositBankFailureAndRedirectRollback(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprint(redirect), func(t *testing.T) {
			h := setupTestSuite(t, nil)
			h.commitBlock(t, h.now)
			h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
				if to.Equals(sinkkeeper.Address()) {
					if redirect {
						return h.bystander, nil
					}

					return nil, fmt.Errorf("bank refused")
				}

				return to, nil
			})

			before := captureSinkBankState(t, h, h.ctx())
			res := h.deliver(t, "", h.deposit(10))
			require.NotZero(t, res.Code)

			if redirect {
				require.Equal(t, types.ErrBalanceMismatch.ABCICode(), res.Code, res.Log)
			} else {
				require.Contains(t, res.Log, "bank refused")
			}

			before.balances[h.sender.String()] = before.balances[h.sender.String()].Sub(coin(10))
			require.Equal(t, before, captureSinkBankState(t, h, h.ctx()))
			require.Empty(t, sinkEvents(t, res.Events))
			require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
			t.Logf("deposit rolled back code=%d log=%s; Sink store, supply, and balances unchanged except the fee", res.Code, res.Log)
		})
	}
}

func TestSinkQueryStatusAndCanonicalIDs(t *testing.T) {
	h := setupReportingTestSuite(t)
	q := h.a.Keepers.Akash.Sink.NewQuerier()
	_, err := q.Transfer(h.ctx(), nil)
	require.Error(t, err)

	for _, id := range []types.ID{
		{},
		{
			Owner:  h.sender.String(),
			Height: 1,
		},
		{
			Owner:    h.sender.String(),
			Height:   -1,
			Sequence: 1,
		},
	} {
		_, err = q.Transfer(h.ctx(), &types.QueryTransferRequest{ID: id})
		require.Error(t, err)
	}

	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	got, err := q.Transfer(h.ctx(), &types.QueryTransferRequest{ID: h.id(1)})
	require.NoError(t, err)
	require.Equal(t, types.StatusTransferPending, got.Transfer.Status)
	h.quorum(t, 1, types.OutcomeVoided)
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	got, err = q.Transfer(h.ctx(), &types.QueryTransferRequest{ID: h.id(1)})
	require.NoError(t, err)
	require.Equal(t, types.StatusRefunded, got.Transfer.Status)
}

func TestSinkRequestBindingAndNativeCacheRollback(t *testing.T) {
	h := setupTestSuite(t, nil)
	res := h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log)
	k := h.a.Keepers.Akash.Sink
	for _, tc := range []struct{ signed, delivered func(*types.MsgSink) }{
		{delivered: func(m *types.MsgSink) { m.Sender = h.bystander.String() }},
		{delivered: func(m *types.MsgSink) { m.Dest = types.SolanaAddress{1}.String() }},
		{delivered: func(m *types.MsgSink) { require.NoError(t, m.Amount.Amount.Unmarshal([]byte("20"))) }},
		{
			signed:    func(m *types.MsgSink) { m.DestType = types.DestTypeInvalid },
			delivered: func(m *types.MsgSink) { m.DestType = types.DestTypeSolana },
		},
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

	before := captureSinkBankState(t, h, h.ctx())
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
	require.Equal(t, before, captureSinkBankState(t, h, h.ctx()))
	redirect = false
	_, err = sinkkeeper.NewDepositDecorator().AnteHandle(ctx, b.GetTx(), false, call)
	require.NoError(t, err)
	require.Equal(t, uint64(1), id.Sequence)
}
