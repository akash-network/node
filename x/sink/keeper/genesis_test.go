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
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
)

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
	key         *storetypes.KVStoreKey
	redirect    sdk.AccAddress
	fail        bool
	restriction func(context.Context, sdk.AccAddress, sdk.AccAddress, sdk.Coins) (sdk.AccAddress, error)
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
	if b.restriction != nil {
		var err error
		to, err = b.restriction(ctx, from, to, coins)
		if err != nil {
			return err
		}
	}

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
	return types.SolanaEvidence{Slot: 10, TransactionSignature: bytes.Repeat([]byte{1}, 64)}
}

func testStore(t *testing.T) (Keeper, sdk.Context, *testBank) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	bankKey := storetypes.NewKVStoreKey("bank")
	tkey := storetypes.NewTransientStoreKey(types.TStoreKey)
	ctx := testutil.DefaultContextWithKeys(map[string]*storetypes.KVStoreKey{types.StoreKey: key, "bank": bankKey}, map[string]*storetypes.TransientStoreKey{types.TStoreKey: tkey}, nil).WithBlockHeight(100).WithBlockTime(time.Unix(86400*20, 0).UTC())
	accounts := genesisAccounts{}
	for n := byte(1); n <= 8; n++ {
		a := authtypes.NewBaseAccountWithAddress(testOwner(n))
		require.NoError(t, a.SetPubKey(secp256k1.GenPrivKey().PubKey()))
		accounts[a.Address] = a
	}

	bank := &testBank{key: bankKey}
	k := NewKeeper(codec.NewProtoCodec(codectypes.NewInterfaceRegistry()), key, tkey, accounts, bank, authtypes.NewModuleAddress("gov").String())
	bank.restriction = k.SendRestriction
	return k, ctx, bank
}

func quorum(p types.ReporterPolicy, outcome types.Outcome) []types.Vote {
	votes := make([]types.Vote, 0, p.Threshold)
	for _, r := range p.Members[:p.Threshold] {
		votes = append(votes, types.Vote{Reporter: r, PolicyVersion: p.Version, Observation: types.Observation{Outcome: outcome, Evidence: testEvidence()}})
	}

	return votes
}

func populatedGenesis() *types.GenesisState {
	g := types.DefaultGenesis()
	g.Params.Reporters = []string{testOwner(2).String(), testOwner(3).String(), testOwner(4).String(), testOwner(5).String()}
	p1 := g.Params.Policy(1)
	p1.Members = []string{testOwner(1).String(), testOwner(2).String(), testOwner(3).String(), testOwner(4).String()}
	p2 := g.Params.Policy(2)
	g.Policies = []types.ReporterPolicy{p1, p2}
	deadline := time.Unix(86400*18, 123).UTC()
	states := []types.SettlementData{
		{Status: types.StatusTransferPending, Round: 1, PolicyVersion: 2},
		{Status: types.StatusTransferPending, Round: 1, PolicyVersion: 1, Hold: types.HoldConflict, Votes: []types.Vote{{Reporter: p1.Members[0], PolicyVersion: 1, Observation: types.Observation{Outcome: types.OutcomeMinted, Evidence: testEvidence()}}, {Reporter: p2.Members[0], PolicyVersion: 2, Observation: types.Observation{Outcome: types.OutcomeVoided, Evidence: testEvidence()}}}},
		{Status: types.StatusTransferPending, Round: 1, PolicyVersion: 2, Hold: types.HoldVeto, Veto: &types.Veto{Signer: p1.Members[0], Reason: "retained after refresh"}},
		{Status: types.StatusMinted, Round: 1, PolicyVersion: 1, Votes: quorum(p1, types.OutcomeMinted)},
		{Status: types.StatusMinted, Round: 1, PolicyVersion: 1, Hold: types.HoldConflict, Votes: append(quorum(p1, types.OutcomeMinted), types.Vote{Reporter: p2.Members[0], PolicyVersion: 2, Observation: types.Observation{Outcome: types.OutcomeVoided, Evidence: testEvidence()}})},
		{Status: types.StatusRefundPending, Round: 1, PolicyVersion: 1, ChallengeEnd: &deadline, Votes: quorum(p1, types.OutcomeVoided)},
		{Status: types.StatusRefundPending, Round: 1, PolicyVersion: 2, ChallengeEnd: &deadline, Votes: quorum(p2, types.OutcomeVoided)},
		{
			Status:        types.StatusRefundPending,
			Round:         2,
			PolicyVersion: 1,
			ChallengeEnd:  &deadline,
			GovernanceEvidence: func() *types.SolanaEvidence {
				e := testEvidence()
				return &e
			}(),
		},
		{Status: types.StatusRefundPending, Round: 1, PolicyVersion: 2, ChallengeEnd: &deadline, Votes: quorum(p2, types.OutcomeVoided), Hold: types.HoldVeto, Veto: &types.Veto{Signer: p2.Members[0], Reason: "review"}},
		{Status: types.StatusRefundPending, Round: 1, PolicyVersion: 2, ChallengeEnd: &deadline, Votes: quorum(p2, types.OutcomeVoided), Hold: types.HoldConflict},
		{Status: types.StatusRefunded, Round: 1, PolicyVersion: 1, Votes: quorum(p1, types.OutcomeVoided)},
		{Status: types.StatusRefunded, Round: 1, PolicyVersion: 1, Votes: append(quorum(p1, types.OutcomeVoided), types.Vote{Reporter: p2.Members[0], PolicyVersion: 2, Observation: types.Observation{Outcome: types.OutcomeMinted, Evidence: testEvidence()}}), Hold: types.HoldConflict},
	}

	sums := map[string]int64{}
	for i, s := range states {
		id := types.ID{Owner: testOwner(byte(i%2 + 6)).String(), Height: 7, Sequence: uint64(i + 1)}
		g.Transfers = append(g.Transfers, types.TransferState{ID: id, Data: types.RecordData{Dest: types.SolanaAddress{1}.String(), Amount: testCoin(10)}, Settlement: s, VoidOrdered: s.GovernanceEvidence != nil})
		if err := g.Totals.Add(s.Status, testCoin(10)); err != nil {
			panic(err)
		}

		sums[id.Owner] += 10
	}

	for n := byte(6); n <= 7; n++ {
		owner := testOwner(n).String()
		g.Accounts = append(g.Accounts, types.GenesisAccount{Owner: owner, Usage: types.AccountUsage{TotalSent: testCoin(sums[owner]), EpochSent: testCoin(sums[owner]), LastEpoch: 19}})
	}

	g.Accounts = append(g.Accounts, types.GenesisAccount{Owner: testOwner(8).String(), Usage: types.AccountUsage{TotalSent: types.ZeroAmount(), EpochSent: types.ZeroAmount(), LastEpoch: 18}})
	g.SendUsage = types.EpochUsage{Epoch: 19, Amount: testCoin(120)}
	g.RefundUsage = types.EpochUsage{Epoch: 19, Amount: testCoin(20)}
	return g
}

func importTestGenesis(t *testing.T, k Keeper, ctx sdk.Context, bank *testBank, g *types.GenesisState) {
	t.Helper()
	require.NoError(t, g.Validate())
	bank.set(ctx, Address(), sdk.NewCoins(g.Totals.PendingAmount.Add(g.Totals.MintedAmount).Add(g.Totals.RefundPendingAmount)))
	require.NoError(t, k.InitGenesis(ctx, g))
}

func TestGenesisPopulatedRecovery(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	exported, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, exported.Transfers, 12)
	require.Len(t, exported.Accounts, 3)
	require.Equal(t, g.SendUsage, exported.SendUsage)
	require.Equal(t, g.RefundUsage, exported.RefundUsage)
	k2, ctx2, bank2 := testStore(t)
	importTestGenesis(t, k2, ctx2, bank2, exported)
	again, err := k2.ExportGenesis(ctx2)
	require.NoError(t, err)
	require.Equal(t, exported, again)
	require.Equal(t, bank.GetAllBalances(ctx, Address()), bank2.GetAllBalances(ctx2, Address()))
	q := k2.NewQuerier()
	state, err := q.State(ctx2, &types.QueryStateRequest{})
	require.NoError(t, err)
	require.True(t, state.SendUsage.Amount.IsZero())
	require.True(t, state.RefundUsage.Amount.IsZero())
	require.NotNil(t, state.RefundHead)
	require.Equal(t, g.Transfers[6].ID, *state.RefundHead)
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
			balance := sdk.NewCoins(g.Totals.PendingAmount.Add(g.Totals.MintedAmount).Add(g.Totals.RefundPendingAmount))
			switch name {
			case "reused height":
				ctx = ctx.WithBlockHeight(7)
			case "zero import height":
				ctx = ctx.WithBlockHeight(0)
			case "duplicate cross-owner position":
				g.Transfers[1].ID.Sequence = g.Transfers[0].ID.Sequence
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
				ctx.KVStore(k.skey).Set([]byte{1, 0}, []byte{1})
			case "missing settlement":
				g.Transfers[0].Settlement = types.SettlementData{}
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
	id := types.ID{Owner: testOwner(1).String(), Height: 1, Sequence: 1}
	require.NoError(t, k.settlements.Set(ctx, id, types.SettlementData{}))
	_, err = k.ExportGenesis(ctx)
	require.ErrorContains(t, err, "orphaned")
}

func TestGenesisCommittedDatabaseRestart(t *testing.T) {
	template, _, _ := testStore(t)
	key := storetypes.NewKVStoreKey(types.StoreKey)
	bankKey := storetypes.NewKVStoreKey("bank")
	tkey := storetypes.NewTransientStoreKey(types.TStoreKey)
	db := dbm.NewMemDB()
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	mount := func() store.CommitMultiStore {
		ms := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
		ms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, nil)
		ms.MountStoreWithDB(bankKey, storetypes.StoreTypeIAVL, nil)
		ms.MountStoreWithDB(tkey, storetypes.StoreTypeTransient, db)
		require.NoError(t, ms.LoadLatestVersion())

		return ms
	}

	first := mount()
	require.NoError(t, first.SetInitialVersion(100))
	header := cmtproto.Header{Height: 100, Time: time.Unix(20*86400, 0).UTC()}
	ctx := sdk.NewContext(first, header, false, log.NewNopLogger())
	bank := &testBank{key: bankKey}
	cdc := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	k := NewKeeper(cdc, key, tkey, template.accounts, bank, template.authority)
	importTestGenesis(t, k, ctx, bank, populatedGenesis())
	before, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	_, err = k.nextID(ctx, testOwner(6))
	require.NoError(t, err)
	require.Equal(t, int64(100), first.Commit().Version)
	second := mount()
	require.Equal(t, int64(100), second.LastCommitID().Version)
	header.Height = 101
	restarted := sdk.NewContext(second, header, false, log.NewNopLogger())
	restored := NewKeeper(cdc, key, tkey, template.accounts, bank, template.authority)
	after, err := restored.ExportGenesis(restarted)
	require.NoError(t, err)
	require.Equal(t, before, after)
	id, err := restored.nextID(restarted, testOwner(6))
	require.NoError(t, err)
	require.Equal(t, uint64(1), id.Sequence)
	require.Equal(t, int64(101), id.Height)
	require.NoError(t, restored.CheckBalance(restarted))
}

func TestGenesisExportRejectsCanonicalCorruption(t *testing.T) {
	for _, kind := range []string{"missing settlement", "bad record", "bad order", "bad totals", "orphan order"} {
		t.Run(kind, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			importTestGenesis(t, k, ctx, bank, g)
			id := g.Transfers[0].ID
			switch kind {
			case "missing settlement":
				require.NoError(t, k.settlements.Remove(ctx, id))
			case "bad record":
				key, err := types.TransferKey(id)
				require.NoError(t, err)
				ctx.KVStore(k.skey).Set(key, []byte{1})
			case "bad order":
				key, err := types.VoidOrderKey(id)
				require.NoError(t, err)
				ctx.KVStore(k.skey).Set(key, []byte{1})
			case "bad totals":
				ctx.KVStore(k.skey).Set(TotalsKey.Bytes(), []byte{})
			case "orphan order":
				require.NoError(t, k.voids.Set(ctx, types.ID{Owner: testOwner(8).String(), Height: 2, Sequence: 1}, types.VoidOrder{}))

			}

			_, err := k.ExportGenesis(ctx)
			require.Error(t, err)
		})
	}
}
