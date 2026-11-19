package handler_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	mocks "pkg.akt.dev/node/v2/testutil/cosmos/mocks"
	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/sink/handler"
	"pkg.akt.dev/node/v2/x/sink/keeper"
)

type testSuite struct {
	*state.TestSuite
	t         *testing.T
	ctx       sdk.Context
	server    types.MsgServer
	sender    sdk.AccAddress
	reporters []string
	dest      string
	balances  map[string]sdk.Coins
}

func setupTestSuite(t *testing.T) *testSuite {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".cache", "sink-test-homes"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(root, 0o700))
	t.Setenv("TMPDIR", root)

	accounts := &mocks.AccountKeeper{}
	ssuite := state.SetupTestSuiteWithKeepers(t, state.Keepers{Account: accounts})
	t.Cleanup(func() { require.NoError(t, ssuite.App().Close()) })
	suite := &testSuite{
		TestSuite: ssuite,
		t:         t,
		ctx:       ssuite.Context().WithBlockHeight(1).WithBlockTime(time.Unix(1_750_000_000, 0).UTC()),
		server:    handler.NewServer(ssuite.SinkKeeper()),
		balances:  make(map[string]sdk.Coins),
	}
	key := secp256k1.GenPrivKeyFromSecret([]byte("local sink sender"))
	suite.sender = sdk.AccAddress(key.PubKey().Address())
	account := authtypes.NewBaseAccount(suite.sender, key.PubKey(), 0, 0)
	accounts.On("GetAccount", mock.Anything, suite.sender).Return(account)
	moduleAccount := ssuite.App().Keepers.Cosmos.Acct.GetModuleAccount(suite.ctx, types.ModuleName)
	accounts.On("GetAccount", mock.Anything, keeper.Address()).Return(moduleAccount)
	accounts.On("GetModuleAccount", mock.Anything, types.ModuleName).Return(moduleAccount)
	for i := 0; i < 5; i++ {
		reporter := secp256k1.GenPrivKeyFromSecret([]byte(fmt.Sprintf("local reporter %d", i)))
		address := sdk.AccAddress(reporter.PubKey().Address())
		suite.reporters = append(suite.reporters, address.String())
		// Reporters are ordinary signing accounts; SetParams rejects module accounts.
		accounts.On("GetAccount", mock.Anything, address).Return(authtypes.NewBaseAccount(address, reporter.PubKey(), 0, 0))
	}
	publicKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	suite.dest = base58.Encode(publicKey)
	suite.balances[suite.sender.String()] = sdk.NewCoins(coin(2_000_000_000))
	suite.balances[keeper.Address().String()] = sdk.NewCoins()
	bank := suite.BankKeeper()
	bank.On("GetAllBalances", mock.Anything, mock.Anything).
		Return(func(_ context.Context, address sdk.AccAddress) sdk.Coins {
			return suite.balances[address.String()]
		})
	bank.On("SendCoinsFromAccountToModule", mock.Anything, suite.sender, types.ModuleName, mock.Anything).
		Return(func(_ context.Context, from sdk.AccAddress, _ string, amount sdk.Coins) error {
			suite.balances[from.String()] = suite.balances[from.String()].Sub(amount...)
			suite.balances[keeper.Address().String()] = suite.balances[keeper.Address().String()].Add(amount...)
			return nil
		})
	bank.On("SendCoinsFromModuleToAccount", mock.Anything, types.ModuleName, suite.sender, mock.Anything).
		Return(func(_ context.Context, _ string, to sdk.AccAddress, amount sdk.Coins) error {
			suite.balances[keeper.Address().String()] = suite.balances[keeper.Address().String()].Sub(amount...)
			suite.balances[to.String()] = suite.balances[to.String()].Add(amount...)
			return nil
		})

	epochs := suite.App().Keepers.Akash.Epochs
	epoch, err := epochs.GetEpoch(suite.ctx, "day")
	require.NoError(t, err)
	epoch.StartTime = suite.ctx.BlockTime()
	require.NoError(t, epochs.RemoveEpoch(suite.ctx, "day"))
	require.NoError(t, epochs.AddEpoch(suite.ctx, epoch))
	require.NoError(t, epochs.BeginBlocker(suite.ctx))
	require.NoError(t, suite.SinkKeeper().BeginBlocker(suite.ctx))
	params := suite.params()
	params.Enabled = true
	params.RefundsEnabled = true
	params.MinAmountPerTransfer = coin(1)
	params.MaxAmountPerTransfer = coin(100)
	params.MaxEpochRefund = coin(100)
	params.Reporters = suite.reporters[:4]
	params.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.75")
	require.NoError(t, suite.update(params))
	return suite
}

func coin(amount int64) sdk.Coin {
	return sdk.NewInt64Coin(sdkutil.DenomUakt, amount)
}

func (s *testSuite) deposit(amount int64) types.ID {
	s.t.Helper()
	msg := &types.MsgSink{
		Sender:   s.sender.String(),
		DestType: types.DestTypeSolana,
		Dest:     s.dest,
		Amount:   coin(amount),
	}
	tx := s.App().TxConfig().NewTxBuilder()
	require.NoError(s.t, tx.SetMsgs(msg))

	ctx, err := keeper.NewDepositDecorator(s.App().Keepers.Akash.Sink).AnteHandle(
		s.ctx,
		tx.GetTx(),
		false,
		func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil },
	)

	require.NoError(s.t, err)
	result, err := s.server.Sink(ctx, msg)
	require.NoError(s.t, err)
	return result.ID
}

func (s *testSuite) transfer(id types.ID) types.TransferState {
	s.t.Helper()
	value, err := s.SinkKeeper().GetTransfer(s.ctx, id)
	require.NoError(s.t, err)
	return value
}

func (s *testSuite) totals() types.Totals {
	s.t.Helper()
	value, err := s.SinkKeeper().GetTotals(s.ctx)
	require.NoError(s.t, err)
	return value
}

func (s *testSuite) params() types.Params {
	s.t.Helper()
	value, err := s.SinkKeeper().GetParams(s.ctx)
	require.NoError(s.t, err)
	return value
}

func (s *testSuite) update(params types.Params) error {
	_, err := s.server.UpdateParams(s.ctx, &types.MsgUpdateParams{
		Authority: s.SinkKeeper().GetAuthority(),
		Params:    params,
	})
	return err
}

func (s *testSuite) reportMsg(reporter int, id types.ID, outcome types.Outcome) *types.MsgReportOutcome {
	s.t.Helper()
	policy, err := s.SinkKeeper().NewQuerier().Reporters(s.ctx, nil)
	require.NoError(s.t, err)
	return &types.MsgReportOutcome{
		Reporter:      s.reporters[reporter],
		ID:            id,
		PolicyVersion: policy.Reporters[0].Version,
		Round:         s.transfer(id).Settlement.Round,
		Observation: types.Observation{
			Outcome:              outcome,
			Slot:                 100,
			TransactionSignature: bytes.Repeat([]byte{1}, 64),
		},
	}
}

func (s *testSuite) report(reporter int, id types.ID, outcome types.Outcome) error {
	_, err := s.server.ReportOutcome(s.ctx, s.reportMsg(reporter, id, outcome))
	return err
}

func (s *testSuite) quorum(id types.ID, outcome types.Outcome) {
	s.t.Helper()
	for i := 0; i < 3; i++ {
		require.NoError(s.t, s.report(i, id, outcome))
	}
}

func (s *testSuite) refund(id types.ID) error {
	_, err := s.server.FinalizeRefund(s.ctx, &types.MsgFinalizeRefund{
		Executor: s.reporters[4],
		ID:       id,
	})
	return err
}

func (s *testSuite) order(id types.ID) error {
	_, err := s.server.RequestVoid(s.ctx, &types.MsgRequestVoid{
		Authority:     s.SinkKeeper().GetAuthority(),
		ID:            id,
		ExpectedRound: s.transfer(id).Settlement.Round,
	})
	return err
}

func (s *testSuite) resolve(id types.ID, decision types.Decision) error {
	round := s.transfer(id).Settlement.Round
	authority := s.SinkKeeper().GetAuthority()
	evidence := types.SolanaEvidence{
		Slot:                 100,
		TransactionSignature: bytes.Repeat([]byte{1}, 64),
	}
	switch decision {
	case types.DecisionRestart:
		_, err := s.server.RestartTransfer(s.ctx, &types.MsgRestartTransfer{
			Authority:     authority,
			ID:            id,
			ExpectedRound: round,
			Reason:        "review",
		})
		return err
	case types.DecisionConfirmMint:
		_, err := s.server.ConfirmMint(s.ctx, &types.MsgConfirmMint{
			Authority:     authority,
			ID:            id,
			ExpectedRound: round,
			Reason:        "review",
			Evidence:      evidence,
		})
		return err
	case types.DecisionConfirmVoid:
		_, err := s.server.ConfirmVoid(s.ctx, &types.MsgConfirmVoid{
			Authority:     authority,
			ID:            id,
			ExpectedRound: round,
			Reason:        "review",
			Evidence:      evidence,
		})
		return err
	default:
		s.t.Fatalf("unsupported test decision %s", decision)
		return nil
	}
}

func (s *testSuite) rotate() {
	params := s.params()
	params.Reporters = s.reporters[1:]
	require.NoError(s.t, s.update(params))
}

func (s *testSuite) mature(id types.ID) {
	s.ctx = s.ctx.WithBlockTime(time.Unix(s.transfer(id).Settlement.ChallengeEnd, 0).UTC())
}

func (s *testSuite) checkSettlementExport() {
	s.t.Helper()
	k := s.SinkKeeper()
	require.NoError(s.t, k.CheckBalance(s.ctx))
	genesis, err := k.ExportGenesis(s.ctx)
	require.NoError(s.t, err)
	require.NoError(s.t, genesis.Validate())
	ctx, _ := s.ctx.WithBlockHeight(s.ctx.BlockHeight() + 1).CacheContext()
	store := ctx.KVStore(s.App().GetKey(types.StoreKey))
	before := make(map[string][]byte)
	it := store.Iterator(nil, nil)
	for ; it.Valid(); it.Next() {
		before[string(it.Key())] = bytes.Clone(it.Value())
	}
	require.NoError(s.t, it.Close())
	for key := range before {
		store.Delete([]byte(key))
	}
	require.NoError(s.t, k.InitGenesis(ctx, genesis))
	after, err := k.ExportGenesis(ctx)
	require.NoError(s.t, err)
	require.Equal(s.t, genesis, after)
	restored := make(map[string][]byte)
	it = store.Iterator(nil, nil)
	for ; it.Valid(); it.Next() {
		restored[string(it.Key())] = bytes.Clone(it.Value())
	}
	require.NoError(s.t, it.Close())
	require.Equal(s.t, before, restored)
	require.NoError(s.t, k.CheckBalance(ctx))
}
