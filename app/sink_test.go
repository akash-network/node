package app_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
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
	"pkg.akt.dev/node/v2/app"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

type sinkHarness struct {
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
	h := &sinkHarness{key: secp256k1.GenPrivKeyFromSecret([]byte("local sink sender")), now: time.Unix(1_750_000_000, 0).UTC()}
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
			{Address: h.sender.String(), Coins: sdk.NewCoins(sdk.NewInt64Coin("uakt", 2_000_000_000), sdk.NewInt64Coin("other", 100))},
			{Address: h.bystander.String(), Coins: sdk.NewCoins(sdk.NewInt64Coin("uakt", 1_000_000_000))},
		} {
			bank.Balances = append(bank.Balances, b)
			bank.Supply = bank.Supply.Add(b.Coins...)
		}
		for _, key := range h.reporters {
			coins := sdk.NewCoins(sdk.NewInt64Coin("uakt", 1_000_000))
			bank.Balances = append(bank.Balances, banktypes.Balance{Address: sdk.AccAddress(key.PubKey().Address()).String(), Coins: coins})
			bank.Supply = bank.Supply.Add(coins...)
		}
		state[banktypes.ModuleName] = cdc.MustMarshalJSON(&bank)
		g := sinktypes.DefaultGenesis()
		g.Params.Enabled = true
		g.Params.MinTransfer = 1 // deposits require a minimum
		for _, key := range h.reporters[:4] {
			g.Params.Reporters = append(g.Params.Reporters, sdk.AccAddress(key.PubKey().Address()).String())
		}
		g.Params.ReporterThreshold = 3
		if change != nil {
			change(g)
		}
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
	return &sinktypes.MsgSink{DestType: 1, Dest: base58.Encode(h.recipient[:]), Amount: sdk.NewInt64Coin("uakt", amount), Sender: h.sender.String()}
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
	b.SetFeeAmount(sdk.NewCoins(sdk.NewInt64Coin("uakt", 10)))
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
	return res.TxResults[0]
}

func (h *sinkHarness) deliver(t *testing.T, memo string, msgs ...sdk.Msg) *abci.ExecTxResult {
	t.Helper()
	return h.deliverBytes(t, h.tx(t, memo, msgs...))
}

func TestSinkSignedDepositAndProof(t *testing.T) {
	h := newSinkHarness(t, nil)
	k := h.a.Keepers.Akash.Sink
	totalSupply := h.a.Keepers.Cosmos.Bank.GetSupply(h.ctx(), "uakt")
	// No launch reserve: escrow starts empty and holds only deposits.
	require.True(t, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sinkkeeper.Address()).IsZero())
	require.NoError(t, k.CheckBalance(h.ctx()))
	bz := h.tx(t, "", h.deposit(123456789))
	// A simulation at height zero cannot create a transfer and never writes state.
	_, _, _ = h.a.Simulate(bz)
	require.Zero(t, k.GetTotals(h.ctx()).PendingCount)
	res := h.deliverBytes(t, bz)
	require.Zero(t, res.Code, res.Log)
	var responseData sdk.TxMsgData
	require.NoError(t, responseData.Unmarshal(res.Data))
	require.Len(t, responseData.MsgResponses, 1)
	require.Equal(t, "/akash.sink.v1.MsgSinkResponse", responseData.MsgResponses[0].TypeUrl)
	var depositResponse sinktypes.MsgSinkResponse
	require.NoError(t, depositResponse.Unmarshal(responseData.MsgResponses[0].Value))
	require.Equal(t, "1", depositResponse.Id)
	ctx := h.ctx()
	entry, ok := k.GetTransfer(ctx, 1)
	require.True(t, ok)
	transfer, err := sinktypes.DecodeTransfer(entry.Record)
	require.NoError(t, err)
	require.Equal(t, sinktypes.Transfer{ID: 1, Sender: [20]byte(h.sender), Recipient: h.recipient, Amount: 123456789, Height: 1}, transfer)
	require.NoError(t, k.CheckBalance(ctx))
	require.Equal(t, uint64(123456789), k.GetTotals(ctx).PendingAmount)
	require.Equal(t, uint32(1), k.GetAccountUsage(ctx, h.sender).PendingCount)
	require.Equal(t, totalSupply, h.a.Keepers.Cosmos.Bank.GetSupply(ctx, "uakt"))
	// Simulation executes the same deposit logic but cannot consume a pending
	// slot or create a durable proof record.
	_, _, err = h.a.Simulate(h.tx(t, "", h.deposit(1)))
	require.NoError(t, err)
	require.Equal(t, uint64(1), k.GetTotals(h.ctx()).PendingCount)
	depositGas := res.GasUsed
	// The SDK disallows proof queries at height <= 1. Use the state committed
	// at height 2, whose app hash belongs in header 3.
	res = h.deliver(t, "", banktypes.NewMsgSend(h.sender, h.bystander, sdk.NewCoins(sdk.NewInt64Coin("uakt", 1))))
	require.Zero(t, res.Code, res.Log)
	appHash := bytes.Clone(h.a.CommitMultiStore().LastCommitID().Hash)
	query, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/store/sink/key", Data: sinktypes.TransferKey(1), Height: 2, Prove: true})
	require.NoError(t, err)
	require.Zero(t, query.Code, query.Log)
	require.Equal(t, entry.Record, query.Value)
	stateQuery, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/akash.sink.v1.Query/State", Data: []byte{}, Height: 2})
	require.NoError(t, err)
	require.Zero(t, stateQuery.Code, stateQuery.Log)
	var state sinktypes.QueryStateResponse
	require.NoError(t, state.Unmarshal(stateQuery.Value))
	require.Equal(t, uint64(123456789), state.Totals.PendingAmount)
	sinkRequest, err := (&sinktypes.QuerySink{Id: depositResponse.Id}).Marshal()
	require.NoError(t, err)
	sinkQuery, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/akash.sink.v1.Query/Sink", Data: sinkRequest, Height: 2})
	require.NoError(t, err)
	require.Zero(t, sinkQuery.Code, sinkQuery.Log)
	var sinkState sinktypes.QuerySinkResponse
	require.NoError(t, sinkState.Unmarshal(sinkQuery.Value))
	require.Equal(t, sinktypes.QuerySinkResponse_transfer_pending, sinkState.Status)
	require.Equal(t, entry.Record, sinkState.Transfer.Record)
	proof, err := commitment.ConvertProofs(query.ProofOps)
	require.NoError(t, err)
	require.NoError(t, proof.VerifyMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(appHash), commitment.NewMerklePath([]byte("sink"), []byte("transfer/1")), entry.Record))
	bad := bytes.Clone(entry.Record)
	bad[61] ^= 1
	require.Error(t, proof.VerifyMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(appHash), commitment.NewMerklePath([]byte("sink"), []byte("transfer/1")), bad))
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)
	t.Logf("real Akash app proof: %d bytes; transfer: %d bytes; tx gas: %d", len(proofBytes), len(entry.Record), depositGas)
	if path := os.Getenv("SINK_PROOF_FIXTURE"); path != "" {
		fixture := map[string]any{"chain_id": "akash-1", "query_height": 2, "header_height": 3, "root": hex.EncodeToString(appHash), "record": hex.EncodeToString(entry.Record), "proof": hex.EncodeToString(proofBytes)}
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

func TestSinkRejectedTransactionsAreAtomic(t *testing.T) {
	h := newSinkHarness(t, nil)
	k := h.a.Keepers.Akash.Sink
	for _, tc := range []struct {
		name, memo string
		msgs       []sdk.Msg
	}{
		{"direct bank deposit", "bad", []sdk.Msg{banktypes.NewMsgSend(h.sender, sinkkeeper.Address(), sdk.NewCoins(sdk.NewInt64Coin("uakt", 10)))}},
		{"extra message", "", []sdk.Msg{h.deposit(10), banktypes.NewMsgSend(h.sender, h.bystander, sdk.NewCoins(sdk.NewInt64Coin("uakt", 1)))}},
		{"two deposits", "", []sdk.Msg{h.deposit(10), h.deposit(10)}},
		{"wrong denom", "", []sdk.Msg{banktypes.NewMsgSend(h.sender, sinkkeeper.Address(), sdk.NewCoins(sdk.NewInt64Coin("other", 1)))}},
		{"mixed denom", "", []sdk.Msg{banktypes.NewMsgSend(h.sender, sinkkeeper.Address(), sdk.NewCoins(sdk.NewInt64Coin("other", 1), sdk.NewInt64Coin("uakt", 1)))}},
		{"oversize", "", []sdk.Msg{h.deposit(int64(sinktypes.DefaultParams().MaxTransfer) + 1)}}, //nolint:gosec // 1M AKT fits int64
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := k.GetTotals(h.ctx())
			res := h.deliver(t, tc.memo, tc.msgs...)
			require.NotZero(t, res.Code, res.Log)
			require.Equal(t, before, k.GetTotals(h.ctx()))
			require.NoError(t, k.CheckBalance(h.ctx()))
		})
	}
	// The sender holds less than the 1M AKT default limit, so validation reaches
	// the bank's insufficient-funds check. Neither the record nor bank state may
	// survive the failure.
	res := h.deliver(t, "", h.deposit(3_000_000_000))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "insufficient funds")
	require.Zero(t, k.GetTotals(h.ctx()).LastTransferId)
	require.Zero(t, k.GetAccountUsage(h.ctx(), h.sender).DailyCount)
	require.NoError(t, k.CheckBalance(h.ctx()))
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(1), k.GetTotals(h.ctx()).LastTransferId)
	// A prior transaction's request capability cannot authorize authz or MultiSend.
	exec := authz.NewMsgExec(h.sender, []sdk.Msg{h.deposit(10)})
	res = h.deliver(t, "", &exec)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "sole top-level MsgSink")
	multi := &banktypes.MsgMultiSend{Inputs: []banktypes.Input{{Address: h.sender.String(), Coins: sdk.NewCoins(sdk.NewInt64Coin("uakt", 10))}}, Outputs: []banktypes.Output{{Address: sinkkeeper.Address().String(), Coins: sdk.NewCoins(sdk.NewInt64Coin("uakt", 10))}}}
	res = h.deliver(t, "", multi)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "is not allowed to receive funds")
	require.Equal(t, uint64(1), k.GetTotals(h.ctx()).PendingCount)
	require.NoError(t, k.CheckBalance(h.ctx()))
}

func TestSinkLimitsAndGovernanceVoid(t *testing.T) {
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) { g.Params.MaxDailyPerAccount = 1; g.Params.MaxPendingPerAccount = 2 })
	k := h.a.Keepers.Akash.Sink
	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	entry, _ := k.GetTransfer(h.ctx(), 1)
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	h.now = h.now.Add(24 * time.Hour)
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	h.now = h.now.Add(24 * time.Hour)
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	res = h.deliver(t, "", &sinktypes.MsgRequestVoid{Authority: h.sender.String(), Id: 1})
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "authority must be governance")
	ctx := h.ctx()
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	require.Error(t, k.RequestVoid(ctx, h.sender.String(), 1))
	require.Error(t, k.RequestVoid(ctx, gov, 999))
	require.NoError(t, k.RequestVoid(ctx, gov, 1))
	require.NoError(t, k.RequestVoid(ctx, gov, 1))
	got, _ := k.GetTransfer(ctx, 1)
	require.True(t, got.VoidOrdered)
	require.Equal(t, entry.Record, got.Record)
	require.Equal(t, sinktypes.VoidValue(1), ctx.KVStore(h.a.GetKey(sinktypes.StoreKey)).Get(sinktypes.VoidKey(1)))
	require.Equal(t, uint64(20), k.GetTotals(ctx).PendingAmount)
	require.NoError(t, k.CheckBalance(ctx))
	server := sinkkeeper.MsgServer{Keeper: k}
	p := k.GetParams(ctx)
	p.Enabled = false
	_, err := server.UpdateParams(ctx, &sinktypes.MsgUpdateParams{Authority: h.sender.String(), Params: p})
	require.Error(t, err)
	_, err = server.UpdateParams(ctx, &sinktypes.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	res = h.deliver(t, "", h.deposit(1))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "disabled")
	ctx = h.ctx()
	exported := k.ExportGenesis(ctx)
	require.NoError(t, exported.Validate())
	require.True(t, exported.Transfers[0].VoidOrdered)
	// Import into a fresh sink store with the exported bank state. Backing is
	// already there.
	cache, _ := ctx.CacheContext()
	s := cache.KVStore(h.a.GetKey(sinktypes.StoreKey))
	it := storetypes.KVStorePrefixIterator(s, nil)
	var keys [][]byte
	for ; it.Valid(); it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}
	require.NoError(t, it.Close())
	for _, key := range keys {
		s.Delete(key)
	}
	require.NoError(t, k.InitGenesis(cache, exported))
	require.Equal(t, exported, k.ExportGenesis(cache))
	require.NoError(t, k.CheckBalance(cache))
}

func TestSinkModuleIngressAndEnable(t *testing.T) {
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) {
		g.Params.Enabled = false
		g.Params.Reporters = nil
		g.Params.ReporterThreshold = 0
	})
	k := h.a.Keepers.Akash.Sink
	ctx := h.ctx()
	bank := h.a.Keepers.Cosmos.Bank
	coins := sdk.NewCoins(sdk.NewInt64Coin("uakt", 1))
	require.Error(t, bank.SendCoinsFromAccountToModule(ctx, h.sender, sinktypes.ModuleName, coins))
	require.Error(t, bank.SendCoinsFromModuleToModule(ctx, minttypes.ModuleName, sinktypes.ModuleName, coins))
	require.Error(t, bank.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, sinkkeeper.Address(), coins))
	require.Error(t, bank.SendCoinsFromModuleToAccount(ctx, sinktypes.ModuleName, h.sender, coins))
	// The sink holds nothing: pending, minted and voided are all zero.
	require.True(t, bank.GetAllBalances(ctx, sinkkeeper.Address()).IsZero())
	require.NoError(t, k.CheckBalance(ctx))
	p := k.GetParams(ctx)
	p.Enabled = true
	require.ErrorContains(t, k.SetParams(ctx, p), "reporters must be configured")
	// Reporters are the only precondition; there is no reserve to fund.
	for _, key := range h.reporters[:4] {
		p.Reporters = append(p.Reporters, sdk.AccAddress(key.PubKey().Address()).String())
	}
	p.ReporterThreshold = 3
	require.NoError(t, k.SetParams(ctx, p))
	require.True(t, k.GetParams(ctx).Enabled)
	require.Zero(t, k.GetTotals(ctx).PendingCount)
	require.NoError(t, k.CheckBalance(ctx))
}

func TestSinkIDNeverWraps(t *testing.T) {
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) { g.Totals.LastTransferId = math.MaxUint64 })
	res := h.deliver(t, "", h.deposit(1))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "counter overflow")
	totals := h.a.Keepers.Akash.Sink.GetTotals(h.ctx())
	require.Equal(t, uint64(math.MaxUint64), totals.LastTransferId)
	require.Zero(t, totals.PendingCount)
	require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
}

func TestSinkMinimumTransfer(t *testing.T) {
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) { g.Params.MinTransfer = 100 })
	k := h.a.Keepers.Akash.Sink
	require.Equal(t, uint64(100), k.GetParams(h.ctx()).MinTransfer)
	before := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender)
	res := h.deliver(t, "", h.deposit(99))
	require.NotZero(t, res.Code, res.Log)
	require.Contains(t, res.Log, "below the minimum transfer of 100 uakt")
	require.Zero(t, k.GetTotals(h.ctx()).LastTransferId)
	require.Zero(t, k.GetAccountUsage(h.ctx(), h.sender).DailyCount)
	// Only the fee left the sender; nothing reached escrow.
	require.Equal(t, before.AmountOf("uakt").SubRaw(10), h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender).AmountOf("uakt"))
	require.NoError(t, k.CheckBalance(h.ctx()))
	res = h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(100), k.GetTotals(h.ctx()).PendingAmount)

	// Governance validates the bound. Deposits keep a minimum while enabled.
	ctx := h.ctx()
	server := sinkkeeper.MsgServer{Keeper: k}
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	p := k.GetParams(ctx)
	p.MinTransfer = p.MaxTransfer + 1
	_, err := server.UpdateParams(ctx, &sinktypes.MsgUpdateParams{Authority: gov, Params: p})
	require.ErrorContains(t, err, "min transfer must not exceed max transfer")
	p.MinTransfer = p.MaxTransfer
	_, err = server.UpdateParams(ctx, &sinktypes.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	res = h.deliver(t, "", h.deposit(int64(p.MaxTransfer)-1)) //nolint:gosec // small test limit
	require.NotZero(t, res.Code, res.Log)
	require.Contains(t, res.Log, "below the minimum")
	p = k.GetParams(h.ctx())
	p.MinTransfer = 0
	_, err = server.UpdateParams(h.ctx(), &sinktypes.MsgUpdateParams{Authority: gov, Params: p})
	require.ErrorContains(t, err, "min transfer must be set")
	p.MinTransfer = 1
	_, err = server.UpdateParams(h.ctx(), &sinktypes.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	res = h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log)

	// Export and import keep the minimum.
	p = k.GetParams(h.ctx())
	p.MinTransfer = 7
	require.NoError(t, k.SetParams(h.ctx(), p))
	ctx = h.ctx()
	exported := k.ExportGenesis(ctx)
	require.Equal(t, uint64(7), exported.Params.MinTransfer)
	cdc := h.a.AppCodec()
	bz, err := cdc.MarshalJSON(&exported)
	require.NoError(t, err)
	var imported sinktypes.GenesisState
	require.NoError(t, cdc.UnmarshalJSON(bz, &imported))
	require.Equal(t, uint64(7), imported.Params.MinTransfer)
	cache, _ := ctx.CacheContext()
	s := cache.KVStore(h.a.GetKey(sinktypes.StoreKey))
	it := storetypes.KVStorePrefixIterator(s, nil)
	var keys [][]byte
	for ; it.Valid(); it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}
	require.NoError(t, it.Close())
	for _, key := range keys {
		s.Delete(key)
	}
	require.NoError(t, k.InitGenesis(cache, imported))
	require.Equal(t, uint64(7), k.GetParams(cache).MinTransfer)
	imported.Params.MinTransfer = imported.Params.MaxTransfer + 1
	require.Error(t, imported.Validate())
}

// The sink is a blocked bank address, so no delegator can make distribution
// pay rewards into it. Such sends would fail the sink restriction and make
// zero-height export panic.
func TestSinkCannotBeWithdrawAddress(t *testing.T) {
	h := newSinkHarness(t, nil)
	require.True(t, h.a.Keepers.Cosmos.Bank.BlockedAddr(sinkkeeper.Address()))
	valAddr := sdk.ValAddress(h.sender)
	pk, err := cryptocodec.FromCmtPubKeyInterface(cmted25519.GenPrivKeyFromSecret([]byte("sink withdraw validator")).PubKey())
	require.NoError(t, err)
	create, err := stakingtypes.NewMsgCreateValidator(valAddr.String(), pk, sdk.NewInt64Coin("uakt", 1_000_000),
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
	res = h.deliver(t, "", distrtypes.NewMsgDepositValidatorRewardsPool(h.sender.String(), valAddr.String(), sdk.NewCoins(sdk.NewInt64Coin("uakt", 100))))
	require.Zero(t, res.Code, res.Log)
	_, err = h.a.ExportAppStateAndValidators(true, nil, nil)
	require.NoError(t, err)
	// Bank messages cannot fund the sink either; only MsgSink deposits can.
	res = h.deliver(t, "", banktypes.NewMsgSend(h.sender, sinkkeeper.Address(), sdk.NewCoins(sdk.NewInt64Coin("uakt", 1))))
	require.NotZero(t, res.Code, res.Log)
	require.Contains(t, res.Log, "is not allowed to receive")
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	require.NoError(t, h.a.Keepers.Akash.Sink.CheckBalance(h.ctx()))
}
