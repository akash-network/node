package app_test

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

func reportingHarness(t *testing.T) *sinkHarness {
	return newSinkHarness(t, func(g *types.GenesisState) {
		g.Params.MaxAmountPerTransfer = coin(100)
		g.Params.MaxEpochRefund = coin(100)
		g.Params.RefundsEnabled = true
	})
}

func observation(outcome types.Outcome) types.Observation {
	return types.Observation{
		Outcome:              outcome,
		Slot:                 100,
		TransactionSignature: bytes.Repeat([]byte{1}, 64),
	}
}

func (h *sinkHarness) reportMsg(t *testing.T, reporter, id int, outcome types.Outcome) *types.MsgReportOutcome {
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

func (h *sinkHarness) report(t *testing.T, reporter, id int, outcome types.Outcome) *abci.ExecTxResult {
	return h.deliverBytes(t, h.txFrom(t, h.reporters[reporter], "", h.reportMsg(t, reporter, id, outcome)))
}

func (h *sinkHarness) quorum(t *testing.T, id int, outcome types.Outcome) {
	t.Helper()

	for i := 0; i < 3; i++ {
		res := h.report(t, i, id, outcome)
		require.Zero(t, res.Code, res.Log)
	}
}

func (h *sinkHarness) refund(t *testing.T, id int) *abci.ExecTxResult {
	return h.deliverBytes(t, h.txFrom(t, h.reporters[4], "", &types.MsgFinalizeRefund{
		Executor: h.reporterAddress(4),
		ID:       h.id(id),
	}))
}

func (h *sinkHarness) transfer(t *testing.T, id int) types.TransferState {
	t.Helper()
	entry, err := h.a.Keepers.Akash.Sink.GetTransfer(h.ctx(), h.id(id))
	require.NoError(t, err)

	return entry
}

func (h *sinkHarness) resolve(t *testing.T, id types.ID, decision types.Decision) error {
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

func (h *sinkHarness) checkSettlementExport(t *testing.T) {
	t.Helper()
	k := h.a.Keepers.Akash.Sink
	require.NoError(t, k.CheckBalance(h.ctx()))
	g, err := k.ExportGenesis(h.ctx())
	require.NoError(t, err)
	require.NoError(t, g.Validate())
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

func TestSinkMintReportsCloseOnce(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	before := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sinkkeeper.Address())
	h.quorum(t, 1, types.OutcomeMinted)
	s := h.transfer(t, 1)
	require.Equal(t, types.StatusMinted, s.Status)
	require.Len(t, s.Settlement.Votes, 3)
	msg := h.reportMsg(t, 0, 1, types.OutcomeMinted)
	msg.Observation.Slot++
	require.NoError(t, h.a.Keepers.Akash.Sink.ReportOutcome(h.ctx(), msg))
	require.Equal(t, s, h.transfer(t, 1))
	require.Equal(t, coin(60), h.totals().MintedAmount)
	require.Equal(t, uint64(1), h.totals().MintedCount)
	require.Zero(t, h.totals().PendingCount)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sinkkeeper.Address()))
	h.checkSettlementExport(t)
}

func TestSinkRefundExactOnceAndBankRollback(t *testing.T) {
	h := reportingHarness(t)
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

func TestSinkVetoConflictAndGovernanceResolution(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.OutcomeVoided)
	k := h.a.Keepers.Akash.Sink
	require.NoError(t, k.VetoVoid(h.ctx(), &types.MsgVetoVoid{
		Signer: h.reporterAddress(0),
		ID:     h.id(1),
		Reason: "check evidence",
	}))
	require.Equal(t, types.HoldVeto, h.transfer(t, 1).Settlement.Hold)
	res = h.report(t, 0, 1, types.OutcomeMinted)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.HoldConflict, h.transfer(t, 1).Settlement.Hold)
	round := h.transfer(t, 1).Settlement.Round
	require.NoError(t, h.resolve(t, h.id(1), types.DecisionRestart))
	require.Equal(t, round+1, h.transfer(t, 1).Settlement.Round)
	require.Empty(t, h.transfer(t, 1).Settlement.Votes)
	require.Empty(t, h.transfer(t, 1).Settlement.VetoBy)
	stale := &types.MsgConfirmMint{
		Authority:     authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		ID:            h.id(1),
		ExpectedRound: round,
		Reason:        "stale proposal",
		Evidence: types.SolanaEvidence{
			Slot:                 100,
			TransactionSignature: bytes.Repeat([]byte{1}, 64),
		},
	}
	require.ErrorIs(t, k.Resolve(h.ctx(), stale), types.ErrStaleReport)
	require.Error(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	require.NoError(t, h.order(h.id(1)))
	require.NoError(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Error(t, h.resolve(t, h.id(1), types.DecisionConfirmMint))
	require.Error(t, h.resolve(t, h.id(1), types.DecisionRestart))
	h.checkSettlementExport(t)
}

func rotate(h *sinkHarness) {
	p := h.params()
	p.Reporters = []string{h.reporterAddress(1), h.reporterAddress(2), h.reporterAddress(3), h.reporterAddress(4)}
	require.NoError(h.t, h.update(p))
}

func TestSinkReporterRotationRestartsAuthorization(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.OutcomeVoided)
	old := h.transfer(t, 1)
	rotate(h)
	require.Nil(t, h.state().RefundHead)
	h.now = time.Unix(old.Settlement.ChallengeEnd, 0).UTC()
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	res = h.report(t, 1, 1, types.OutcomeVoided)
	require.Zero(t, res.Code, res.Log)
	s := h.transfer(t, 1)
	require.Equal(t, types.StatusTransferPending, s.Status)
	require.Len(t, s.Settlement.Votes, 1)
	require.Zero(t, s.Settlement.ChallengeEnd)

	for _, r := range []int{2, 3} {
		res = h.report(t, r, 1, types.OutcomeVoided)
		require.Zero(t, res.Code, res.Log)
	}

	s = h.transfer(t, 1)
	require.Equal(t, types.StatusRefundPending, s.Status)
	require.Greater(t, s.Settlement.ChallengeEnd, old.Settlement.ChallengeEnd)
	h.checkSettlementExport(t)
}

func TestSinkRotationCannotRemoveHold(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.OutcomeVoided)
	k := h.a.Keepers.Akash.Sink
	require.NoError(t, k.VetoVoid(h.ctx(), &types.MsgVetoVoid{
		Signer: h.reporterAddress(0),
		ID:     h.id(1),
		Reason: "keep me",
	}))
	rotate(h)
	res = h.report(t, 1, 1, types.OutcomeVoided)
	require.Zero(t, res.Code, res.Log)
	s := h.transfer(t, 1)
	require.Equal(t, types.HoldVeto, s.Settlement.Hold)
	require.Equal(t, "keep me", s.Settlement.VetoReason)
	require.Positive(t, s.Settlement.ChallengeEnd)
	require.Equal(t, types.StatusRefundPending, s.Status)
	h.checkSettlementExport(t)
}

func TestSinkTerminalEvidenceSurvivesRotation(t *testing.T) {
	for _, outcome := range []types.Outcome{types.OutcomeMinted, types.OutcomeVoided} {
		t.Run(outcome.String(), func(t *testing.T) {
			h := reportingHarness(t)
			res := h.deliver(t, "", h.deposit(60))
			require.Zero(t, res.Code, res.Log)
			h.quorum(t, 1, outcome)

			if outcome == types.OutcomeVoided {
				h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
				res = h.refund(t, 1)
				require.Zero(t, res.Code, res.Log)
			}

			before := h.transfer(t, 1)
			rotate(h)
			opposite := types.OutcomeVoided
			if outcome == types.OutcomeVoided {
				opposite = types.OutcomeMinted
			}

			res = h.report(t, 4, 1, opposite)
			require.Zero(t, res.Code, res.Log)
			after := h.transfer(t, 1)
			require.Equal(t, before.Status, after.Status)
			require.Equal(t, before.Settlement.Policy.Version, after.Settlement.Policy.Version)
			require.Equal(t, before.Settlement.Votes, after.Settlement.Votes[:len(before.Settlement.Votes)])
			require.Equal(t, types.HoldConflict, after.Settlement.Hold)
			disputes, err := h.a.Keepers.Akash.Sink.NewQuerier().Disputes(h.ctx(), nil)
			require.NoError(t, err)
			require.Len(t, disputes.Transfers, 1)

			if outcome == types.OutcomeVoided {
				require.NoError(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
				require.Equal(t, types.StatusRefunded, h.transfer(t, 1).Status)
			}

			h.checkSettlementExport(t)
		})
	}
}

func TestSinkRefundPauseAndEpochCap(t *testing.T) {
	h := reportingHarness(t)
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

func TestSinkGovernanceWhenReportersUnavailable(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	require.NoError(t, h.order(h.id(1)))
	require.NoError(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	version := h.transfer(t, 1).Settlement.Policy.Version
	p := h.params()
	p.Enabled = false
	p.Reporters = nil
	require.NoError(t, h.update(p))
	require.Equal(t, version, h.transfer(t, 1).Settlement.Policy.Version)
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	h.checkSettlementExport(t)
}
