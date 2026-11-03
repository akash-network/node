package keeper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	epochstypes "pkg.akt.dev/go/node/epochs/v1beta1"
	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	epochskeeper "pkg.akt.dev/node/v2/x/epochs/keeper"
)

type testEpochs struct {
	info epochstypes.EpochInfo
	err  error
}

func (e testEpochs) GetEpoch(_ sdk.Context, _ string) (epochstypes.EpochInfo, error) {
	return e.info, e.err
}

type genesisAccounts map[string]sdk.AccountI

func (a genesisAccounts) GetAccount(_ context.Context, addr sdk.AccAddress) sdk.AccountI {
	return a[addr.String()]
}

func (a genesisAccounts) SetAccount(_ context.Context, acc sdk.AccountI) {
	a[acc.GetAddress().String()] = acc
}

func (a genesisAccounts) GetModuleAccount(_ context.Context, name string) sdk.ModuleAccountI {
	return authtypes.NewEmptyModuleAccount(name)
}

type testBank struct {
	key      *storetypes.KVStoreKey
	redirect sdk.AccAddress
	fail     bool
}

func (b *testBank) set(ctx context.Context, addr sdk.AccAddress, coins sdk.Coins) {
	raw, err := json.Marshal(coins)
	if err != nil {
		panic(err)
	}

	sdk.UnwrapSDKContext(ctx).KVStore(b.key).Set(addr, raw)
}

func (b *testBank) GetAllBalances(ctx context.Context, addr sdk.AccAddress) sdk.Coins {
	raw := sdk.UnwrapSDKContext(ctx).KVStore(b.key).Get(addr)
	if raw == nil {
		return sdk.NewCoins()
	}

	var coins sdk.Coins
	if err := json.Unmarshal(raw, &coins); err != nil {
		panic(err)
	}

	return coins
}

func (b *testBank) send(ctx context.Context, from, to sdk.AccAddress, coins sdk.Coins) error {
	if b.redirect != nil {
		to = b.redirect
	}

	balance := b.GetAllBalances(ctx, from)
	if !balance.IsAllGTE(coins) {
		return fmt.Errorf("insufficient funds")
	}

	b.set(ctx, from, balance.Sub(coins...))
	b.set(ctx, to, b.GetAllBalances(ctx, to).Add(coins...))

	if b.fail {
		return fmt.Errorf("late bank failure")
	}

	return nil
}

func (b *testBank) SendCoinsFromAccountToModule(ctx context.Context, from sdk.AccAddress, to string, coins sdk.Coins) error {
	return b.send(ctx, from, authtypes.NewModuleAddress(to), coins)
}

func (b *testBank) SendCoinsFromModuleToAccount(ctx context.Context, from string, to sdk.AccAddress, coins sdk.Coins) error {
	return b.send(ctx, authtypes.NewModuleAddress(from), to, coins)
}

func testOwner(n byte) sdk.AccAddress {
	return sdk.AccAddress(bytes.Repeat([]byte{n}, 20))
}

func testCoin(n int64) sdk.Coin {
	return sdk.NewInt64Coin(sdkutil.DenomUakt, n)
}

func testEvidence() types.SolanaEvidence {
	return types.SolanaEvidence{
		Slot:                 10,
		TransactionSignature: bytes.Repeat([]byte{1}, 64),
	}
}

func testStore(t *testing.T) (*keeper, sdk.Context, *testBank) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	bankKey := storetypes.NewKVStoreKey("bank")
	epochKey := storetypes.NewKVStoreKey(epochstypes.StoreKey)
	tkey := storetypes.NewTransientStoreKey(types.TStoreKey)
	ctx := testutil.DefaultContextWithKeys(map[string]*storetypes.KVStoreKey{types.StoreKey: key, "bank": bankKey, epochstypes.StoreKey: epochKey}, map[string]*storetypes.TransientStoreKey{types.TStoreKey: tkey}, nil).WithBlockHeight(100).WithBlockTime(time.Unix(86400*20, 0).UTC())
	accounts := genesisAccounts{}
	for n := byte(1); n <= 8; n++ {
		a := authtypes.NewBaseAccountWithAddress(testOwner(n))
		require.NoError(t, a.SetPubKey(secp256k1.GenPrivKey().PubKey()))
		accounts[a.Address] = a
	}

	bank := &testBank{key: bankKey}
	cdc := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	epochs := epochskeeper.NewKeeper(cdc, runtime.NewKVStoreService(epochKey))
	epochs.SetHooks(epochstypes.NewMultiEpochHooks())
	require.NoError(t, epochs.AddEpoch(ctx, epochstypes.EpochInfo{
		ID:                      "day",
		StartTime:               ctx.BlockTime().Add(-19 * 10 * time.Minute),
		Duration:                10 * time.Minute,
		CurrentEpoch:            20,
		CurrentEpochStartTime:   ctx.BlockTime(),
		CurrentEpochStartHeight: 100,
		EpochCountingStarted:    true,
	}))

	k := NewKeeper(cdc, key, tkey, accounts, bank, epochs, authtypes.NewModuleAddress("gov").String()).(*keeper)
	return k, ctx, bank
}

func quorum(p types.ReporterPolicy, outcome types.Outcome) []types.Vote {
	votes := make([]types.Vote, 0, p.Threshold)
	for _, r := range p.Members[:p.Threshold] {
		votes = append(votes, types.Vote{
			Reporter:      r,
			PolicyVersion: p.Version,
			Observation: types.Observation{
				Outcome:              outcome,
				Slot:                 testEvidence().Slot,
				TransactionSignature: testEvidence().TransactionSignature,
			},
		})
	}

	return votes
}

func populatedGenesis() *types.GenesisState {
	g := types.DefaultGenesis()

	g.Params.Reporters = []string{
		testOwner(2).String(),
		testOwner(3).String(),
		testOwner(4).String(),
		testOwner(5).String(),
	}

	policyMembers := make([]string, len(g.Params.Reporters))
	copy(policyMembers, g.Params.Reporters)

	p1 := types.ReporterPolicy{
		Version:   1,
		Members:   policyMembers,
		Threshold: g.Params.ReportersThreshold.Clone(),
	}

	p2 := types.ReporterPolicy{
		Version:   2,
		Members:   policyMembers,
		Threshold: g.Params.ReportersThreshold.Clone(),
	}

	g.Policies = []types.ReporterPolicy{p1, p2}
	g.PolicySequence = 2
	deadline := int64(86400 * 18)
	observe := func(outcome types.Outcome) types.Observation {
		e := testEvidence()
		return types.Observation{
			Outcome:              outcome,
			Slot:                 e.Slot,
			TransactionSignature: e.TransactionSignature,
		}
	}

	opposite := func(p types.ReporterPolicy, outcome types.Outcome) types.Vote {
		return types.Vote{
			Reporter:      p.Members[0],
			PolicyVersion: p.Version,
			Observation:   observe(outcome),
		}
	}

	governance := observe(types.OutcomeVoided)
	records := []types.GenesisRecord{
		{
			Status: types.StatusTransferPending,
			Settlement: types.SettlementData{
				Round:  1,
				Policy: p2,
			},
		},
		{
			Status: types.StatusTransferPending,
			Settlement: types.SettlementData{
				Round:  1,
				Policy: p1,
				Hold:   types.HoldConflict,
				Votes:  []types.Vote{opposite(p1, types.OutcomeMinted), opposite(p2, types.OutcomeVoided)},
			},
		},
		{
			Status: types.StatusVoided,
			Settlement: types.SettlementData{
				Round:  1,
				Policy: p1,
				Hold:   types.HoldConflict,
				Votes:  []types.Vote{opposite(p1, types.OutcomeMinted), opposite(p2, types.OutcomeVoided)},
				VoidAuthorization: &types.VoidAuthorization{
					RequestedHeight: 7,
					RequestedRound:  1,
				},
			},
		},
		{
			Status: types.StatusMinted,
			Settlement: types.SettlementData{
				Round:  1,
				Policy: p1,
				Votes:  quorum(p1, types.OutcomeMinted),
			},
		},
		{
			Status: types.StatusMinted,
			Settlement: types.SettlementData{
				Round:  1,
				Policy: p1,
				Votes:  append(quorum(p1, types.OutcomeMinted), opposite(p2, types.OutcomeVoided)),
				Hold:   types.HoldConflict,
			},
		},
		{
			Status: types.StatusRefundPending,
			Settlement: types.SettlementData{
				Round:        1,
				Policy:       p1,
				ChallengeEnd: deadline,
				Votes:        quorum(p1, types.OutcomeVoided),
			},
		},
		{
			Status: types.StatusRefundPending,
			Settlement: types.SettlementData{
				Round:        1,
				Policy:       p2,
				ChallengeEnd: deadline,
				Votes:        quorum(p2, types.OutcomeVoided),
			},
		},
		{
			Status: types.StatusRefundPending,
			Settlement: types.SettlementData{
				Round:                 2,
				Policy:                p1,
				ChallengeEnd:          deadline,
				GovernanceObservation: &governance,
				VoidAuthorization: &types.VoidAuthorization{
					RequestedHeight: 7,
					RequestedRound:  1,
				},
			},
		},
		{
			Status: types.StatusRefundPending,
			Settlement: types.SettlementData{
				Round:             1,
				Policy:            p2,
				ChallengeEnd:      deadline,
				Votes:             quorum(p2, types.OutcomeVoided),
				Hold:              types.HoldVeto,
				VetoBy:            p1.Members[0],
				VetoReason:        "retained after rotation",
				VetoPolicyVersion: 1,
			},
		},
		{
			Status: types.StatusRefundPending,
			Settlement: types.SettlementData{
				Round:        1,
				Policy:       p2,
				ChallengeEnd: deadline,
				Votes:        append(quorum(p2, types.OutcomeVoided), opposite(p2, types.OutcomeMinted)),
				Hold:         types.HoldConflict,
			},
		},
		{
			Status: types.StatusRefunded,
			Settlement: types.SettlementData{
				Round:  1,
				Policy: p1,
				Votes:  quorum(p1, types.OutcomeVoided),
			},
		},
		{
			Status: types.StatusRefunded,
			Settlement: types.SettlementData{
				Round:  1,
				Policy: p1,
				Votes:  append(quorum(p1, types.OutcomeVoided), opposite(p2, types.OutcomeMinted)),
				Hold:   types.HoldConflict,
			},
		},
	}

	sums := map[string]int64{}
	for i, record := range records {
		record.ID = types.ID{
			Owner:    testOwner(byte(i%2 + 6)).String(),
			Height:   7,
			Sequence: uint64(i + 1),
		}

		record.Data = types.RecordData{
			Dest:   types.SolanaAddress{1}.String(),
			Amount: testCoin(10),
		}

		g.Records = append(g.Records, record)
		if err := g.Totals.Add(record.Status, record.Data.Amount); err != nil {
			panic(err)
		}

		sums[record.ID.Owner] += 10
	}

	for n := byte(6); n <= 7; n++ {
		owner := testOwner(n).String()
		g.Accounts = append(g.Accounts, types.GenesisAccount{
			Owner: owner,
			Usage: types.AccountUsage{
				TotalSent: testCoin(sums[owner]),
				EpochSent: testCoin(sums[owner]),
				LastEpoch: 19,
			},
		})
	}

	g.Accounts = append(g.Accounts, types.GenesisAccount{
		Owner: testOwner(8).String(),
		Usage: types.AccountUsage{
			TotalSent: types.ZeroAmount(),
			EpochSent: types.ZeroAmount(),
			LastEpoch: 18,
		},
	})

	g.SendUsage = types.EpochUsage{
		Epoch:  19,
		Amount: testCoin(120),
	}

	g.RefundUsage = types.EpochUsage{
		Epoch:  19,
		Amount: testCoin(20),
	}

	return g
}

func importTestGenesis(t *testing.T, k Keeper, ctx sdk.Context, bank *testBank, g *types.GenesisState) {
	t.Helper()
	require.NoError(t, g.Validate())
	bank.set(ctx, Address(), sdk.NewCoins(g.Totals.PendingAmount.Add(g.Totals.MintedAmount).Add(g.Totals.VoidedAmount).Add(g.Totals.RefundPendingAmount)))
	require.NoError(t, k.InitGenesis(ctx, g))
}

func nativeQuotaGenesis() *types.GenesisState {
	g := populatedGenesis()
	g.Params.Enabled = true
	g.Params.RefundsEnabled = true
	g.Params.MinAmountPerTransfer = testCoin(1)
	g.Params.MaxAmountPerTransfer = testCoin(10)
	g.Params.MaxEpochAmountPerAccount = testCoin(60)
	g.Params.MaxTransferAmountPerEpoch = testCoin(60)
	g.Params.MaxEpochRefund = testCoin(20)
	g.Accounts[0].Usage.LastEpoch = 57
	g.Accounts[1].Usage.LastEpoch = 56
	g.Accounts[2].Usage.LastEpoch = 55
	g.SendUsage = types.EpochUsage{
		Epoch:  57,
		Amount: testCoin(60),
	}

	g.RefundUsage = types.EpochUsage{
		Epoch:  57,
		Amount: testCoin(20),
	}

	return g
}

func TestGenesisPopulatedRecovery(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := nativeQuotaGenesis()
	epochs := k.epochs.(epochskeeper.Keeper)
	native, err := epochs.GetEpoch(ctx, "day")
	require.NoError(t, err)
	native.CurrentEpoch = 57
	require.NoError(t, epochs.RemoveEpoch(ctx, "day"))
	require.NoError(t, epochs.AddEpoch(ctx, native))
	importTestGenesis(t, k, ctx, bank, g)
	exported, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, exported.Records, 12)
	require.Len(t, exported.Accounts, 3)
	require.Equal(t, g.SendUsage, exported.SendUsage)
	require.Equal(t, g.RefundUsage, exported.RefundUsage)
	k2, ctx2, bank2 := testStore(t)
	epochs2 := k2.epochs.(epochskeeper.Keeper)
	require.NoError(t, epochs2.RemoveEpoch(ctx2, "day"))
	require.NoError(t, epochs2.AddEpoch(ctx2, native))
	importTestGenesis(t, k2, ctx2, bank2, exported)
	native2, err := epochs2.GetEpoch(ctx2, "day")
	require.NoError(t, err)
	require.Equal(t, native, native2)

	for _, transfer := range exported.Records {
		key, err := TransferKey(transfer.ID)
		require.NoError(t, err)
		require.Equal(t, ctx.KVStore(k.skey).Get(key), ctx2.KVStore(k2.skey).Get(key))
	}

	again, err := k2.ExportGenesis(ctx2)
	require.NoError(t, err)
	require.Equal(t, exported, again)
	require.Equal(t, bank.GetAllBalances(ctx, Address()), bank2.GetAllBalances(ctx2, Address()))
	q := k2.NewQuerier()
	state, err := q.State(ctx2, &types.QueryStateRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(57), state.Epoch)
	require.Equal(t, g.SendUsage, state.SendUsage)
	require.Equal(t, g.RefundUsage, state.RefundUsage)
	stale, err := q.Account(ctx2, &types.QueryAccountRequest{Owner: testOwner(7).String()})
	require.NoError(t, err)
	require.True(t, stale.Usage.EpochSent.IsZero())
	raw, err := k2.GetAccountUsage(ctx2, testOwner(7))
	require.NoError(t, err)
	require.Equal(t, int64(56), raw.LastEpoch)
	require.Equal(t, testCoin(60), raw.EpochSent)
	require.NotNil(t, state.RefundHead)
	require.Equal(t, g.Records[6].ID, *state.RefundHead)
	disputes, err := q.Disputes(ctx2, nil)
	require.NoError(t, err)
	require.Len(t, disputes.Transfers, 6)
	id, err := k2.nextID(ctx2, testOwner(6))
	require.NoError(t, err)
	require.Equal(t, uint64(1), id.Sequence)
	require.Equal(t, int64(100), id.Height)
	require.NoError(t, k2.BeginBlocker(ctx2))
	id, err = k2.nextID(ctx2.WithBlockHeight(101), testOwner(7))
	require.NoError(t, err)
	require.Equal(t, uint64(1), id.Sequence)
	require.Error(t, k2.InitGenesis(ctx2, exported))
}

func TestGenesisRejectsLostIdentityOrBacking(t *testing.T) {
	for _, name := range []string{"reused height", "zero import height", "duplicate cross-owner position", "future send", "future refund", "future account", "missing backing", "extra denom", "legacy store", "missing settlement", "wrong totals", "missing account"} {
		t.Run(name, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			balance := sdk.NewCoins(g.Totals.PendingAmount.Add(g.Totals.MintedAmount).Add(g.Totals.VoidedAmount).Add(g.Totals.RefundPendingAmount))
			switch name {
			case "reused height":
				ctx = ctx.WithBlockHeight(7)
			case "zero import height":
				ctx = ctx.WithBlockHeight(0)
			case "duplicate cross-owner position":
				g.Records[1].ID.Sequence = g.Records[0].ID.Sequence
			case "future send":
				g.SendUsage.Epoch = 21
			case "future refund":
				g.RefundUsage.Epoch = 21
			case "future account":
				g.Accounts[2].Usage.LastEpoch = 21
			case "missing backing":
				balance = sdk.NewCoins()
			case "extra denom":
				balance = balance.Add(sdk.NewInt64Coin("other", 1))
			case "legacy store":
				ctx.KVStore(k.skey).Set([]byte{0x11, 0}, []byte{1})
			case "missing settlement":
				g.Records[0].Settlement = types.SettlementData{}
			case "wrong totals":
				g.Totals.PendingCount++
			case "missing account":
				g.Accounts = g.Accounts[1:]
			}

			bank.set(ctx, Address(), balance)
			beforeEvents := ctx.EventManager().Events()
			require.Error(t, k.InitGenesis(ctx, g))
			require.Equal(t, balance, bank.GetAllBalances(ctx, Address()))
			require.Equal(t, beforeEvents, ctx.EventManager().Events())
			initialized, err := k.initialized(ctx)
			require.NoError(t, err)
			require.False(t, initialized)
		})
	}
}

func TestGenesisZeroHeightEmptyAndCorruption(t *testing.T) {
	k, ctx, bank := testStore(t)
	importTestGenesis(t, k, ctx.WithBlockHeight(0), bank, types.DefaultGenesis())
	_, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	id := types.ID{
		Owner:    testOwner(1).String(),
		Height:   1,
		Sequence: 1,
	}

	require.NoError(t, k.settlements.Set(ctx, *types.NewSettlementID(types.StatusTransferPending, id), types.SettlementData{}))
	_, err = k.ExportGenesis(ctx)
	require.ErrorContains(t, err, "orphaned")
}

func TestGenesisCommittedDatabaseRestart(t *testing.T) {
	template, _, _ := testStore(t)
	key := storetypes.NewKVStoreKey(types.StoreKey)
	bankKey := storetypes.NewKVStoreKey("bank")
	epochKey := storetypes.NewKVStoreKey(epochstypes.StoreKey)
	tkey := storetypes.NewTransientStoreKey(types.TStoreKey)
	db := dbm.NewMemDB()
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	mount := func() store.CommitMultiStore {
		ms := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
		ms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, nil)
		ms.MountStoreWithDB(bankKey, storetypes.StoreTypeIAVL, nil)
		ms.MountStoreWithDB(epochKey, storetypes.StoreTypeIAVL, nil)
		ms.MountStoreWithDB(tkey, storetypes.StoreTypeTransient, db)
		require.NoError(t, ms.LoadLatestVersion())

		return ms
	}

	first := mount()
	require.NoError(t, first.SetInitialVersion(100))
	header := cmtproto.Header{
		Height: 100,
		Time:   time.Unix(20*86400, 0).UTC(),
	}

	ctx := sdk.NewContext(first, header, false, log.NewNopLogger())
	bank := &testBank{key: bankKey}
	cdc := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	epochs := epochskeeper.NewKeeper(cdc, runtime.NewKVStoreService(epochKey))
	native := epochstypes.EpochInfo{
		ID:                      "day",
		Duration:                10 * time.Minute,
		StartTime:               header.Time.Add(-56 * 10 * time.Minute),
		CurrentEpochStartTime:   header.Time,
		CurrentEpochStartHeight: 100,
		CurrentEpoch:            57,
		EpochCountingStarted:    true,
	}

	require.NoError(t, epochs.AddEpoch(ctx, native))
	k := NewKeeper(cdc, key, tkey, template.accounts, bank, epochs, template.authority).(*keeper)
	importTestGenesis(t, k, ctx, bank, nativeQuotaGenesis())
	before, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	_, err = k.nextID(ctx, testOwner(6))
	require.NoError(t, err)
	require.Equal(t, int64(100), first.Commit().Version)
	second := mount()
	require.Equal(t, int64(100), second.LastCommitID().Version)
	header.Height = 101
	restarted := sdk.NewContext(second, header, false, log.NewNopLogger())
	restoredEpochs := epochskeeper.NewKeeper(cdc, runtime.NewKVStoreService(epochKey))
	restoredEpochs.SetHooks(epochstypes.NewMultiEpochHooks())
	restored := NewKeeper(cdc, key, tkey, template.accounts, bank, restoredEpochs, template.authority).(*keeper)
	gotEpoch, err := restoredEpochs.GetEpoch(restarted, "day")
	require.NoError(t, err)
	require.Equal(t, native, gotEpoch)
	after, err := restored.ExportGenesis(restarted)
	require.NoError(t, err)
	require.Equal(t, before, after)
	id, err := restored.nextID(restarted, testOwner(6))
	require.NoError(t, err)
	require.Equal(t, uint64(1), id.Sequence)
	require.Equal(t, int64(101), id.Height)
	require.NoError(t, restored.CheckBalance(restarted))
	state, err := restored.NewQuerier().State(restarted, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(57), state.Epoch)
	require.Equal(t, before.SendUsage, state.SendUsage)
	require.Equal(t, before.RefundUsage, state.RefundUsage)
	err = restored.FinalizeRefund(restarted, &types.MsgFinalizeRefund{
		Executor: testOwner(6).String(),
		ID:       nativeQuotaGenesis().Records[6].ID,
	})
	require.ErrorIs(t, err, types.ErrRefundCapReached)
	restarted = restarted.WithBlockTime(native.CurrentEpochStartTime.Add(native.Duration + time.Nanosecond))
	require.NoError(t, restoredEpochs.BeginBlocker(restarted))
	state, err = restored.NewQuerier().State(restarted, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(58), state.Epoch)
	require.True(t, state.SendUsage.Amount.IsZero())
	require.True(t, state.RefundUsage.Amount.IsZero())
	require.NoError(t, restored.FinalizeRefund(restarted, &types.MsgFinalizeRefund{
		Executor: testOwner(6).String(),
		ID:       *state.RefundHead,
	}))
}

func TestGenesisExportRejectsCanonicalCorruption(t *testing.T) {
	for _, kind := range []string{"missing settlement", "bad record", "bad settlement", "bad totals", "orphan settlement"} {
		t.Run(kind, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			importTestGenesis(t, k, ctx, bank, g)
			id := g.Records[0].ID
			switch kind {
			case "missing settlement":
				require.NoError(t, k.settlements.Remove(ctx, *types.NewSettlementID(types.StatusTransferPending, id)))
			case "bad record":
				key, err := TransferKey(id)
				require.NoError(t, err)
				ctx.KVStore(k.skey).Set(key, []byte{1})
			case "bad settlement":
				key, err := SettlementKey(*types.NewSettlementID(types.StatusTransferPending, id))
				require.NoError(t, err)
				ctx.KVStore(k.skey).Set(key, []byte{1})
			case "bad totals":
				ctx.KVStore(k.skey).Set(TotalsKey.Bytes(), []byte{})
			case "orphan settlement":
				require.NoError(t, k.settlements.Set(ctx, *types.NewSettlementID(types.StatusTransferPending, types.ID{
					Owner:    testOwner(8).String(),
					Height:   2,
					Sequence: 1,
				}), types.SettlementData{}))

			}

			_, err := k.ExportGenesis(ctx)
			require.Error(t, err)
		})
	}
}
