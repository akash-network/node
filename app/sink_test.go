package app_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	cmted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/codec"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsign "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	sinktypes "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
	"pkg.akt.dev/node/v2/app"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

type sinkHarness struct {
	t         *testing.T
	ids       []sinktypes.ID
	a         *app.AkashApp
	key       *secp256k1.PrivKey
	reporters []*secp256k1.PrivKey
	sender    sdk.AccAddress
	bystander sdk.AccAddress // an ordinary funded account with no sink role
	recipient [32]byte
	height    int64
	now       time.Time
	// signBytes holds the bytes signed for the most recently built transaction.
	signBytes []byte
}

func newSinkHarness(t *testing.T, change func(*sinktypes.GenesisState)) *sinkHarness {
	t.Helper()
	h := &sinkHarness{t: t, key: secp256k1.GenPrivKeyFromSecret([]byte("local sink sender")), now: time.Unix(1_750_000_000, 0).UTC()}
	h.sender = sdk.AccAddress(h.key.PubKey().Address())
	h.bystander = sdk.AccAddress(secp256k1.GenPrivKeyFromSecret([]byte("local bystander")).PubKey().Address())
	for i := 0; i < 5; i++ {
		h.reporters = append(h.reporters, secp256k1.GenPrivKeyFromSecret([]byte(fmt.Sprintf("local reporter %d", i))))
	}

	copy(h.recipient[:], ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32)).Public().(ed25519.PublicKey))
	h.a = app.Setup(app.WithHome(t.TempDir()), app.WithGenesis(func(cdc codec.Codec) app.GenesisState {
		state := app.GenesisStateWithValSet(cdc)

		// Isolate bridge supply accounting from ordinary per-block inflation.
		mint := minttypes.DefaultGenesisState()
		mint.Minter.Inflation = sdkmath.LegacyZeroDec()
		mint.Minter.AnnualProvisions = sdkmath.LegacyZeroDec()
		mint.Params.InflationMin = sdkmath.LegacyZeroDec()
		mint.Params.InflationMax = sdkmath.LegacyZeroDec()
		mint.Params.InflationRateChange = sdkmath.LegacyZeroDec()
		state[minttypes.ModuleName] = cdc.MustMarshalJSON(mint)
		var auth authtypes.GenesisState
		cdc.MustUnmarshalJSON(state[authtypes.ModuleName], &auth)
		accounts, err := authtypes.UnpackAccounts(auth.Accounts)
		require.NoError(t, err)
		accounts = append(accounts, authtypes.NewBaseAccountWithAddress(h.sender), authtypes.NewBaseAccountWithAddress(h.bystander))
		for _, key := range h.reporters {
			accounts = append(accounts, authtypes.NewBaseAccountWithAddress(sdk.AccAddress(key.PubKey().Address())))
		}

		state[authtypes.ModuleName] = cdc.MustMarshalJSON(authtypes.NewGenesisState(auth.Params, accounts))
		var bank banktypes.GenesisState
		cdc.MustUnmarshalJSON(state[banktypes.ModuleName], &bank)

		for _, b := range []banktypes.Balance{
			{Address: h.sender.String(), Coins: sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 2_000_000_000), sdk.NewInt64Coin("other", 100))},
			{Address: h.bystander.String(), Coins: sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1_000_000_000))},
		} {
			bank.Balances = append(bank.Balances, b)
			bank.Supply = bank.Supply.Add(b.Coins...)
		}

		for _, key := range h.reporters {
			coins := sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1_000_000))
			bank.Balances = append(bank.Balances, banktypes.Balance{Address: sdk.AccAddress(key.PubKey().Address()).String(), Coins: coins})
			bank.Supply = bank.Supply.Add(coins...)
		}

		state[banktypes.ModuleName] = cdc.MustMarshalJSON(&bank)
		g := sinktypes.DefaultGenesis()
		g.Params.DepositsEnabled = true
		g.Params.MinAmountPerTransfer = coin(1)
		for _, key := range h.reporters[:4] {
			g.Params.Reporters = append(g.Params.Reporters, sdk.AccAddress(key.PubKey().Address()).String())
		}

		g.Params.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.75")
		if change != nil {
			change(g)
		}

		g.Policies = []sinktypes.ReporterPolicy{g.Params.Policy(1)}
		state[sinktypes.ModuleName] = cdc.MustMarshalJSON(g)
		return state
	}))

	t.Cleanup(func() { require.NoError(t, h.a.Close()) })

	return h
}

func (h *sinkHarness) ctx() sdk.Context {
	if h.height == 0 {
		return h.a.NewContext(false)
	}

	return h.a.NewUncachedContext(false, cmtproto.Header{Height: h.height, Time: h.now})
}

func (h *sinkHarness) deposit(amount int64) *sinktypes.MsgSink {
	return &sinktypes.MsgSink{DestType: sinktypes.DestTypeSolana, Dest: base58.Encode(h.recipient[:]), Amount: sdk.NewInt64Coin(sdkutil.DenomUakt, amount), Sender: h.sender.String()}
}

func (h *sinkHarness) tx(t *testing.T, memo string, msgs ...sdk.Msg) []byte {
	return h.txFrom(t, h.key, memo, msgs...)
}

func (h *sinkHarness) txFrom(t *testing.T, key *secp256k1.PrivKey, memo string, msgs ...sdk.Msg) []byte {
	return h.txFromMode(t, key, signing.SignMode_SIGN_MODE_DIRECT, memo, msgs...)
}

func (h *sinkHarness) txFromMode(t *testing.T, key *secp256k1.PrivKey, mode signing.SignMode, memo string, msgs ...sdk.Msg) []byte {
	t.Helper()
	sender := sdk.AccAddress(key.PubKey().Address())
	cfg := h.a.TxConfig()
	b := cfg.NewTxBuilder()
	require.NoError(t, b.SetMsgs(msgs...))
	b.SetMemo(memo)
	b.SetGasLimit(500_000)
	b.SetFeeAmount(sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 10)))
	account := h.a.Keepers.Cosmos.Acct.GetAccount(h.ctx(), sender)
	sig := signing.SignatureV2{PubKey: key.PubKey(), Data: &signing.SingleSignatureData{SignMode: mode}, Sequence: account.GetSequence()}
	require.NoError(t, b.SetSignatures(sig))
	signBytes, err := authsign.GetSignBytesAdapter(context.Background(), cfg.SignModeHandler(), mode,
		authsign.SignerData{Address: sender.String(), ChainID: "akash-1", AccountNumber: account.GetAccountNumber(), Sequence: account.GetSequence(), PubKey: key.PubKey()}, b.GetTx())
	require.NoError(t, err)
	h.signBytes = signBytes
	signature, err := key.Sign(signBytes)
	require.NoError(t, err)
	sig.Data.(*signing.SingleSignatureData).Signature = signature
	require.NoError(t, b.SetSignatures(sig))
	bz, err := cfg.TxEncoder()(b.GetTx())
	require.NoError(t, err)

	return bz
}

func (h *sinkHarness) deliverBytes(t *testing.T, bz []byte) *abci.ExecTxResult {
	t.Helper()
	h.height++
	h.now = h.now.Add(time.Second)
	res, err := h.a.FinalizeBlock(&abci.RequestFinalizeBlock{Height: h.height, Time: h.now, Txs: [][]byte{bz}})
	require.NoError(t, err)
	require.Len(t, res.TxResults, 1)
	_, err = h.a.Commit()
	require.NoError(t, err)
	h.capture(t, res.TxResults[0])

	return res.TxResults[0]
}

func (h *sinkHarness) deliver(t *testing.T, memo string, msgs ...sdk.Msg) *abci.ExecTxResult {
	t.Helper()
	return h.deliverBytes(t, h.tx(t, memo, msgs...))
}

func coin(n int64) sdk.Coin {
	return sdk.NewInt64Coin(sdkutil.DenomUakt, n)
}

func sinkParams(t *testing.T, k sinkkeeper.Keeper, ctx sdk.Context) sinktypes.Params {
	t.Helper()
	v, err := k.GetParams(ctx)
	require.NoError(t, err)

	return v
}

func sinkTotals(t *testing.T, k sinkkeeper.Keeper, ctx sdk.Context) sinktypes.Totals {
	t.Helper()
	v, err := k.GetTotals(ctx)
	require.NoError(t, err)

	return v
}

func (h *sinkHarness) params() sinktypes.Params {
	return sinkParams(h.t, h.a.Keepers.Akash.Sink, h.ctx())
}

func (h *sinkHarness) totals() sinktypes.Totals {
	return sinkTotals(h.t, h.a.Keepers.Akash.Sink, h.ctx())
}

func (h *sinkHarness) state() *sinktypes.QueryStateResponse {
	v, err := h.a.Keepers.Akash.Sink.NewQuerier().State(h.ctx(), nil)
	require.NoError(h.t, err)

	return v
}

func (h *sinkHarness) usage(owner sdk.AccAddress) sinktypes.AccountUsage {
	u, err := h.a.Keepers.Akash.Sink.GetAccountUsage(h.ctx(), owner)
	require.NoError(h.t, err)

	return u
}

func (h *sinkHarness) id(n int) sinktypes.ID {
	require.LessOrEqual(h.t, n, len(h.ids))
	return h.ids[n-1]
}

func (h *sinkHarness) capture(t *testing.T, res *abci.ExecTxResult) {
	if res.Code != 0 {
		return
	}

	var data sdk.TxMsgData
	require.NoError(t, data.Unmarshal(res.Data))

	for _, r := range data.MsgResponses {
		if r.TypeUrl == "/akash.sink.v1.MsgSinkResponse" {
			var out sinktypes.MsgSinkResponse
			require.NoError(t, out.Unmarshal(r.Value))
			h.ids = append(h.ids, out.ID)
		}
	}
}

func (h *sinkHarness) update(p sinktypes.Params) error {
	return h.a.Keepers.Akash.Sink.UpdateParams(h.ctx(), &sinktypes.MsgUpdateParams{Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(), Params: p})
}

func (h *sinkHarness) order(id sinktypes.ID) error {
	return h.a.Keepers.Akash.Sink.RequestVoid(h.ctx(), &sinktypes.MsgRequestVoid{Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(), ID: id})
}

func TestSinkSignedDepositAndProof(t *testing.T) {
	h := newSinkHarness(t, nil)
	k := h.a.Keepers.Akash.Sink
	totalSupply := h.a.Keepers.Cosmos.Bank.GetSupply(h.ctx(), sdkutil.DenomUakt)

	// No launch reserve: escrow starts empty and holds only deposits.
	require.True(t, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sinkkeeper.Address()).IsZero())
	require.NoError(t, k.CheckBalance(h.ctx()))
	bz := h.tx(t, "", h.deposit(123456789))

	// A simulation at height zero cannot create a transfer and never writes state.
	_, _, _ = h.a.Simulate(bz)
	require.Zero(t, h.totals().PendingCount)
	res := h.deliverBytes(t, bz)
	require.Zero(t, res.Code, res.Log)
	var responseData sdk.TxMsgData
	require.NoError(t, responseData.Unmarshal(res.Data))
	require.Len(t, responseData.MsgResponses, 1)
	require.Equal(t, "/akash.sink.v1.MsgSinkResponse", responseData.MsgResponses[0].TypeUrl)
	var depositResponse sinktypes.MsgSinkResponse
	require.NoError(t, depositResponse.Unmarshal(responseData.MsgResponses[0].Value))
	require.Equal(t, sinktypes.ID{Owner: h.sender.String(), Height: 1, Sequence: 1}, depositResponse.ID)
	ctx := h.ctx()
	entry, err := k.GetTransfer(ctx, depositResponse.ID)
	require.NoError(t, err)
	require.Equal(t, sinktypes.RecordData{Dest: base58.Encode(h.recipient[:]), Amount: coin(123456789)}, entry.Data)
	record, err := sinktypes.RecordDataValue.Encode(entry.Data)
	require.NoError(t, err)
	proofKey, err := sinktypes.TransferKey(entry.ID)
	require.NoError(t, err)
	require.NoError(t, k.CheckBalance(ctx))
	require.Equal(t, coin(123456789), sinkTotals(t, k, ctx).PendingAmount)
	require.Equal(t, coin(123456789), h.usage(h.sender).TotalSent)
	require.Equal(t, totalSupply, h.a.Keepers.Cosmos.Bank.GetSupply(ctx, sdkutil.DenomUakt))

	// Simulation executes the same deposit logic but cannot consume a pending
	// slot or create a durable proof record.
	_, _, err = h.a.Simulate(h.tx(t, "", h.deposit(1)))
	require.NoError(t, err)
	require.Equal(t, uint64(1), h.totals().PendingCount)
	depositGas := res.GasUsed

	// The SDK disallows proof queries at height <= 1. Use the state committed
	// at height 2, whose app hash belongs in header 3.
	res = h.deliver(t, "", banktypes.NewMsgSend(h.sender, h.bystander, sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1))))
	require.Zero(t, res.Code, res.Log)
	appHash := bytes.Clone(h.a.CommitMultiStore().LastCommitID().Hash)
	query, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/store/sink/key", Data: proofKey, Height: 2, Prove: true})
	require.NoError(t, err)
	require.Zero(t, query.Code, query.Log)
	require.Equal(t, record, query.Value)
	stateQuery, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/akash.sink.v1.Query/State", Data: []byte{}, Height: 2})
	require.NoError(t, err)
	require.Zero(t, stateQuery.Code, stateQuery.Log)
	var state sinktypes.QueryStateResponse
	require.NoError(t, state.Unmarshal(stateQuery.Value))
	require.Equal(t, coin(123456789), state.Totals.PendingAmount)
	sinkRequest, err := (&sinktypes.QueryTransferRequest{ID: depositResponse.ID}).Marshal()
	require.NoError(t, err)
	sinkQuery, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/akash.sink.v1.Query/Transfer", Data: sinkRequest, Height: 2})
	require.NoError(t, err)
	require.Zero(t, sinkQuery.Code, sinkQuery.Log)
	var sinkState sinktypes.QueryTransferResponse
	require.NoError(t, sinkState.Unmarshal(sinkQuery.Value))
	require.Equal(t, sinktypes.StatusTransferPending, sinkState.Transfer.Settlement.Status)
	require.Equal(t, entry.Data, sinkState.Transfer.Data)
	proof, err := commitment.ConvertProofs(query.ProofOps)
	require.NoError(t, err)
	require.NoError(t, proof.VerifyMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(appHash), commitment.NewMerklePath([]byte("sink"), proofKey), record))
	bad := bytes.Clone(record)
	bad[33] ^= 1
	require.Error(t, proof.VerifyMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(appHash), commitment.NewMerklePath([]byte("sink"), proofKey), bad))
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)
	t.Logf("real Akash app proof: %d bytes; transfer: %d bytes; tx gas: %d", len(proofBytes), len(record), depositGas)

	if path := os.Getenv("SINK_PROOF_FIXTURE"); path != "" {
		fixture := map[string]any{"chain_id": "akash-1", "query_height": 2, "header_height": 3, "root": hex.EncodeToString(appHash), "key": hex.EncodeToString(proofKey), "id": entry.ID, "record": hex.EncodeToString(record), "proof": hex.EncodeToString(proofBytes)}
		transferReply, err := (&sinktypes.QueryTransferResponse{Transfer: entry}).Marshal()
		require.NoError(t, err)
		fixture["state_response"] = hex.EncodeToString(stateQuery.Value)
		fixture["transfer_response"] = hex.EncodeToString(transferReply)
		var signers []map[string]any
		for _, key := range h.reporters[:3] {
			addr := sdk.AccAddress(key.PubKey().Address())
			account := h.a.Keepers.Cosmos.Acct.GetAccount(h.ctx(), addr)
			signers = append(signers, map[string]any{"address": addr.String(), "account_number": fmt.Sprint(account.GetAccountNumber()), "sequence": account.GetSequence()})
		}

		fixture["reporters"] = signers
		data, err := json.MarshalIndent(fixture, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o644)) //nolint:gosec // fixture for another process
	}
}

func TestSinkCannotBeWithdrawAddress(t *testing.T) {
	h := newSinkHarness(t, nil)
	require.True(t, h.a.Keepers.Cosmos.Bank.BlockedAddr(sinkkeeper.Address()))
	valAddr := sdk.ValAddress(h.sender)
	pk, err := cryptocodec.FromCmtPubKeyInterface(cmted25519.GenPrivKeyFromSecret([]byte("sink withdraw validator")).PubKey())
	require.NoError(t, err)
	create, err := stakingtypes.NewMsgCreateValidator(valAddr.String(), pk, sdk.NewInt64Coin(sdkutil.DenomUakt, 1_000_000),
		stakingtypes.NewDescription("sink-test", "", "", "", ""),
		stakingtypes.NewCommissionRates(sdkmath.LegacyNewDecWithPrec(10, 2), sdkmath.LegacyNewDecWithPrec(20, 2), sdkmath.LegacyNewDecWithPrec(1, 2)),
		sdkmath.OneInt())
	require.NoError(t, err)
	res := h.deliver(t, "", create)
	require.Zero(t, res.Code, res.Log)
	res = h.deliver(t, "", distrtypes.NewMsgSetWithdrawAddress(h.sender, sinkkeeper.Address()))
	require.NotZero(t, res.Code, res.Log)
	require.Contains(t, res.Log, "is not allowed to receive")
	withdraw, err := h.a.Keepers.Cosmos.Distr.GetDelegatorWithdrawAddr(h.ctx(), h.sender)
	require.NoError(t, err)
	require.Equal(t, h.sender, withdraw)

	// Accrue commission and rewards, then withdraw everything at zero height.
	res = h.deliver(t, "", distrtypes.NewMsgDepositValidatorRewardsPool(h.sender.String(), valAddr.String(), sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 100))))
	require.Zero(t, res.Code, res.Log)
	_, err = h.a.ExportAppStateAndValidators(true, nil, nil)
	require.NoError(t, err)

	// Bank messages cannot fund the sink either; only MsgSink deposits can.
	res = h.deliver(t, "", banktypes.NewMsgSend(h.sender, sinkkeeper.Address(), sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1))))
	require.NotZero(t, res.Code, res.Log)
	require.Contains(t, res.Log, "is not allowed to receive")
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
}

func TestSinkRejectedTransactionsAreAtomic(t *testing.T) {
	h := newSinkHarness(t, nil)
	k := h.a.Keepers.Akash.Sink
	exec := authz.NewMsgExec(h.sender, []sdk.Msg{h.deposit(10)})
	for _, msgs := range [][]sdk.Msg{
		{banktypes.NewMsgSend(h.sender, sinkkeeper.Address(), sdk.NewCoins(coin(10)))},
		{h.deposit(10), h.deposit(10)},
		{h.deposit(10), banktypes.NewMsgSend(h.sender, h.bystander, sdk.NewCoins(coin(1)))},
		{&exec},
		{&banktypes.MsgMultiSend{Inputs: []banktypes.Input{{Address: h.sender.String(), Coins: sdk.NewCoins(coin(10))}}, Outputs: []banktypes.Output{{Address: sinkkeeper.Address().String(), Coins: sdk.NewCoins(coin(10))}}}},
		{h.deposit(3_000_000_000)},
	} {
		before := h.totals()
		res := h.deliver(t, "", msgs...)
		require.NotZero(t, res.Code)
		require.Equal(t, before, h.totals())
		require.NoError(t, k.CheckBalance(h.ctx()))
	}

	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(1), h.id(1).Sequence)
}

func TestSinkLimitsAndGovernanceVoid(t *testing.T) {
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) {
		g.Params.MaxAmountPerTransfer = coin(10)
		g.Params.MaxEpochAmountPerAccount = coin(10)
	})

	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	res = h.deliver(t, "", h.deposit(1))
	require.NotZero(t, res.Code)
	h.now = h.now.Add(24 * time.Hour)
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	res = h.deliver(t, "", &sinktypes.MsgRequestVoid{Authority: h.sender.String(), ID: h.id(1)})
	require.NotZero(t, res.Code)
	require.NoError(t, h.order(h.id(1)))
	require.NoError(t, h.order(h.id(1)))
	got, err := h.a.Keepers.Akash.Sink.GetTransfer(h.ctx(), h.id(1))
	require.NoError(t, err)
	require.True(t, got.VoidOrdered)
	require.Equal(t, sinktypes.StatusTransferPending, got.Settlement.Status)
	key, err := sinktypes.VoidOrderKey(h.id(1))
	require.NoError(t, err)
	require.Equal(t, []byte{2}, h.ctx().KVStore(h.a.GetKey(sinktypes.StoreKey)).Get(key))
	p := h.params()
	p.DepositsEnabled = false
	require.NoError(t, h.update(p))
	res = h.deliver(t, "", h.deposit(1))
	require.NotZero(t, res.Code)
	h.checkSettlementExport(t)
}

func TestSinkModuleIngressAndEnable(t *testing.T) {
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) {
		g.Params.DepositsEnabled = false
		g.Params.Reporters = nil
	})

	k := h.a.Keepers.Akash.Sink
	ctx := h.ctx()
	bank := h.a.Keepers.Cosmos.Bank
	coins := sdk.NewCoins(coin(1))
	require.Error(t, bank.SendCoinsFromAccountToModule(ctx, h.sender, sinktypes.ModuleName, coins))
	require.Error(t, bank.SendCoinsFromModuleToModule(ctx, minttypes.ModuleName, sinktypes.ModuleName, coins))
	require.Error(t, bank.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, sinkkeeper.Address(), coins))
	require.Error(t, bank.SendCoinsFromModuleToAccount(ctx, sinktypes.ModuleName, h.sender, coins))
	p := h.params()
	p.DepositsEnabled = true
	require.Error(t, h.update(p))

	for _, key := range h.reporters[:4] {
		p.Reporters = append(p.Reporters, sdk.AccAddress(key.PubKey().Address()).String())
	}

	require.NoError(t, h.update(p))
	require.True(t, h.params().DepositsEnabled)
	require.NoError(t, k.CheckBalance(ctx))
}

func TestSinkMinimumTransfer(t *testing.T) {
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) { g.Params.MinAmountPerTransfer = coin(100) })
	before := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender)
	res := h.deliver(t, "", h.deposit(99))
	require.NotZero(t, res.Code)
	require.Zero(t, h.totals().PendingCount)
	require.Equal(t, before.AmountOf(sdkutil.DenomUakt).SubRaw(10), h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender).AmountOf(sdkutil.DenomUakt))
	res = h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	p := h.params()
	p.MinAmountPerTransfer = p.MaxAmountPerTransfer.Add(coin(1))
	require.Error(t, h.update(p))
	p.MinAmountPerTransfer = coin(0)
	require.Error(t, h.update(p))
	p.MinAmountPerTransfer = coin(7)
	require.NoError(t, h.update(p))
	h.checkSettlementExport(t)
}

func TestSinkSameBlockAllocationAndFailedDeposit(t *testing.T) {
	h := newSinkHarness(t, nil)
	transactions := make([][]byte, 0, 3)
	transactions = append(transactions, h.tx(t, "", h.deposit(3_000_000_000)))
	for _, key := range h.reporters[3:5] {
		msg := h.deposit(10)
		msg.Sender = sdk.AccAddress(key.PubKey().Address()).String()
		transactions = append(transactions, h.txFrom(t, key, "", msg))
	}

	h.height = 1
	h.now = h.now.Add(time.Second)
	block, err := h.a.FinalizeBlock(&abci.RequestFinalizeBlock{Height: 1, Time: h.now, Txs: transactions})
	require.NoError(t, err)
	require.Len(t, block.TxResults, 3)
	require.NotZero(t, block.TxResults[0].Code)

	for _, result := range block.TxResults[1:] {
		require.Zero(t, result.Code, result.Log)
		h.capture(t, result)
	}

	_, err = h.a.Commit()
	require.NoError(t, err)
	require.Equal(t, uint64(1), h.id(1).Sequence)
	require.Equal(t, uint64(2), h.id(2).Sequence)
	require.NotEqual(t, h.id(1).Owner, h.id(2).Owner)
	require.Equal(t, h.id(1).Height, h.id(2).Height)
	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(1), h.id(3).Sequence)
	require.Equal(t, int64(2), h.id(3).Height)
}

func TestSinkIDNeverWraps(t *testing.T) {
	h := newSinkHarness(t, nil)
	res := h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log)
	ctx := h.ctx().WithBlockHeight(h.height + 1)
	key := h.a.GetTKey(sinktypes.TStoreKey)
	ctx.TransientStore(key).Set(sinkkeeper.SequenceKey.Bytes(), binary.BigEndian.AppendUint64(nil, math.MaxUint64))
	msg := h.deposit(1)
	builder := h.a.TxConfig().NewTxBuilder()
	require.NoError(t, builder.SetMsgs(msg))
	_, err := sinkkeeper.NewDepositDecorator().AnteHandle(ctx, builder.GetTx(), false, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		_, err := h.a.Keepers.Akash.Sink.Deposit(ctx, msg)
		return ctx, err
	})
	require.ErrorIs(t, err, sinktypes.ErrOverflow)
	require.Equal(t, uint64(1), h.totals().PendingCount)
	require.NoError(t, h.a.Keepers.Akash.Sink.BeginBlocker(ctx))
}
