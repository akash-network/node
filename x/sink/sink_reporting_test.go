package sink_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"

	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

func setupReportingTestSuite(t *testing.T) *testSuite {
	return setupTestSuite(t, func(p *types.Params) {
		p.MaxAmountPerTransfer = coin(100)
		p.MaxEpochRefund = coin(100)
		p.RefundsEnabled = true
	})
}

func observation(outcome types.Outcome) types.Observation {
	return types.Observation{
		Outcome:              outcome,
		Slot:                 100,
		TransactionSignature: bytes.Repeat([]byte{1}, 64),
	}
}

func (h *testSuite) reportMsg(t *testing.T, reporter, id int, outcome types.Outcome) *types.MsgReportOutcome {
	t.Helper()
	entry, err := h.a.Keepers.Akash.Sink.GetTransfer(h.ctx(), h.id(id))
	require.NoError(t, err)

	return &types.MsgReportOutcome{
		Reporter:      h.reporterAddress(reporter),
		ID:            h.id(id),
		PolicyVersion: h.policy().Reporters[0].Version,
		Round:         entry.Settlement.Round,
		Observation:   observation(outcome),
	}
}

func (h *testSuite) report(t *testing.T, reporter, id int, outcome types.Outcome) *abci.ExecTxResult {
	return h.deliverBytes(t, h.txFrom(t, h.reporters[reporter], "", h.reportMsg(t, reporter, id, outcome)))
}

func (h *testSuite) quorum(t *testing.T, id int, outcome types.Outcome) {
	t.Helper()

	for i := 0; i < 3; i++ {
		res := h.report(t, i, id, outcome)
		require.Zero(t, res.Code, res.Log)
	}
}

func (h *testSuite) refund(t *testing.T, id int) *abci.ExecTxResult {
	return h.deliverBytes(t, h.txFrom(t, h.reporters[4], "", &types.MsgFinalizeRefund{
		Executor: h.reporterAddress(4),
		ID:       h.id(id),
	}))
}

func (h *testSuite) transfer(t *testing.T, id int) types.TransferState {
	t.Helper()
	entry, err := h.a.Keepers.Akash.Sink.GetTransfer(h.ctx(), h.id(id))
	require.NoError(t, err)

	return entry
}

func (h *testSuite) resolve(t *testing.T, id types.ID, decision types.Decision) error {
	t.Helper()
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	transfer, err := h.a.Keepers.Akash.Sink.GetTransfer(h.ctx(), id)
	require.NoError(t, err)
	round := transfer.Settlement.Round
	var msg types.Resolution
	switch decision {
	case types.DecisionRestart:
		msg = &types.MsgRestartTransfer{
			Authority:     authority,
			ID:            id,
			ExpectedRound: round,
			Reason:        "review",
		}
	case types.DecisionConfirmMint:
		msg = &types.MsgConfirmMint{
			Authority:     authority,
			ID:            id,
			ExpectedRound: round,
			Reason:        "review",
			Evidence: types.SolanaEvidence{
				Slot:                 100,
				TransactionSignature: bytes.Repeat([]byte{1}, 64),
			},
		}
	case types.DecisionConfirmVoid:
		msg = &types.MsgConfirmVoid{
			Authority:     authority,
			ID:            id,
			ExpectedRound: round,
			Reason:        "review",
			Evidence: types.SolanaEvidence{
				Slot:                 100,
				TransactionSignature: bytes.Repeat([]byte{1}, 64),
			},
		}
	}

	_, err = h.a.MsgServiceRouter().Handler(msg)(h.ctx(), msg)
	return err
}

func (h *testSuite) checkSettlementExport(t *testing.T) {
	t.Helper()
	k := h.a.Keepers.Akash.Sink
	require.NoError(t, k.CheckBalance(h.ctx()))
	g, err := k.ExportGenesis(h.ctx())
	require.NoError(t, err)
	require.NoError(t, g.ValidateBasic())
	cache, _ := h.ctx().CacheContext()
	cache = cache.WithBlockHeight(h.height + 1)
	before := storeEntries(t, cache.KVStore(h.a.GetKey(types.StoreKey)), nil)
	clearSinkStore(t, h, cache)
	require.NoError(t, k.InitGenesis(cache, g))
	again, err := k.ExportGenesis(cache)
	require.NoError(t, err)
	require.Equal(t, g, again)
	require.Equal(t, before, storeEntries(t, cache.KVStore(h.a.GetKey(types.StoreKey)), nil))
	require.NoError(t, k.CheckBalance(cache))
}

func TestSinkRefundExactOnceAndBankRollback(t *testing.T) {
	h := setupReportingTestSuite(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.OutcomeVoided)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	fail := true
	h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
		if fail && from.Equals(sinkkeeper.Address()) {
			return nil, types.ErrInternal.Wrap("injected bank failure")
		}

		return to, nil
	})

	before := h.totals()
	s := h.transfer(t, 1)
	balance := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender)
	executor := sdk.AccAddress(h.reporters[4].PubKey().Address())
	bankState := captureSinkBankState(t, h, h.ctx(), executor)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "injected bank failure")
	bankState.balances[executor.String()] = bankState.balances[executor.String()].Sub(coin(10))
	require.Equal(t, bankState, captureSinkBankState(t, h, h.ctx(), executor))
	require.Equal(t, before, h.totals())
	require.Equal(t, s, h.transfer(t, 1))
	require.Equal(t, balance, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender))
	t.Logf("refund rolled back code=%d log=%s; usage, queue, settlement, and transfer balances retained", res.Code, res.Log)
	fail = false
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.StatusRefunded, h.transfer(t, 1).Status)
	require.Equal(t, balance.Add(coin(60)), h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender))
	require.Equal(t, coin(60), h.totals().RefundedAmount)
	require.Equal(t, uint64(1), h.totals().RefundedCount)
	paid := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, paid, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender))
	h.checkSettlementExport(t)
}

func rotate(h *testSuite) {
	p := h.params()
	p.Reporters = []string{h.reporterAddress(1), h.reporterAddress(2), h.reporterAddress(3), h.reporterAddress(4)}
	require.NoError(h.t, h.update(p))
}

func TestSinkRefundPauseAndEpochCap(t *testing.T) {
	h := setupReportingTestSuite(t)
	for i := 0; i < 2; i++ {
		res := h.deliver(t, "", h.deposit(60))
		require.Zero(t, res.Code, res.Log)
		h.quorum(t, i+1, types.OutcomeVoided)
	}

	h.now = time.Unix(h.transfer(t, 2).Settlement.ChallengeEnd, 0).UTC()
	h.catchUpQuotaEpoch(t)
	p := h.params()
	p.RefundsEnabled = false
	require.NoError(t, h.update(p))
	res := h.refund(t, 1)
	require.NotZero(t, res.Code)
	p.RefundsEnabled = true
	require.NoError(t, h.update(p))
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	res = h.refund(t, 2)
	require.NotZero(t, res.Code)
	h.now = h.now.Add(24 * time.Hour)
	res = h.refund(t, 2)
	require.Zero(t, res.Code, res.Log)
	h.checkSettlementExport(t)
}
