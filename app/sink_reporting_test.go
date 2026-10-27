package app_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	storetypes "cosmossdk.io/store/types"
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
		g.Params.MaxTransfer = 100
		g.Params.DailyRefundCap = 100
		g.Params.ChallengePeriodSeconds = types.DefaultChallengePeriodSeconds
		g.Params.RefundsEnabled = true
	})
}

func observation(outcome types.Outcome) types.Observation {
	return types.Observation{Outcome: outcome, Slot: 100, TransactionSignature: bytes.Repeat([]byte{byte(outcome) + 1}, 64)}
}

func (h *sinkHarness) reportMsg(t *testing.T, reporter int, id uint64, outcome types.Outcome) *types.MsgReportOutcome {
	t.Helper()
	entry, ok := h.a.Keepers.Akash.Sink.GetTransfer(h.ctx(), id)
	require.True(t, ok)
	return &types.MsgReportOutcome{Reporter: sdk.AccAddress(h.reporters[reporter].PubKey().Address()).String(), Id: id,
		ReporterEpoch: h.a.Keepers.Akash.Sink.GetTotals(h.ctx()).ReporterEpoch,
		Round:         entry.Settlement.Round, Observation: observation(outcome)}
}

func (h *sinkHarness) report(t *testing.T, reporter int, id uint64, outcome types.Outcome) *abci.ExecTxResult {
	t.Helper()
	return h.deliverBytes(t, h.txFrom(t, h.reporters[reporter], "", h.reportMsg(t, reporter, id, outcome)))
}

func (h *sinkHarness) quorum(t *testing.T, id uint64, outcome types.Outcome) {
	t.Helper()
	for i := 0; i < 3; i++ {
		res := h.report(t, i, id, outcome)
		require.Zero(t, res.Code, res.Log)
	}
}

func (h *sinkHarness) refund(t *testing.T, id uint64) *abci.ExecTxResult {
	t.Helper()
	key := h.reporters[4] // Not in the reporter set; finalization is permissionless.
	return h.deliverBytes(t, h.txFrom(t, key, "", &types.MsgFinalizeRefund{Executor: sdk.AccAddress(key.PubKey().Address()).String(), Id: id}))
}

func (h *sinkHarness) settlement(t *testing.T, id uint64) types.Settlement {
	t.Helper()
	entry, ok := h.a.Keepers.Akash.Sink.GetTransfer(h.ctx(), id)
	require.True(t, ok)
	return entry.Settlement
}

func (h *sinkHarness) resolve(t *testing.T, id uint64, decision types.Decision) error {
	t.Helper()
	msg := &types.MsgResolveTransfer{Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(), Id: id, Decision: decision, Reason: "local governance review"}
	if decision == types.Decision_DECISION_CONFIRM_MINT {
		value := observation(types.Outcome_OUTCOME_MINTED)
		msg.Observation = &value
	}
	if decision == types.Decision_DECISION_CONFIRM_VOID {
		value := observation(types.Outcome_OUTCOME_VOIDED)
		msg.Observation = &value
	}
	// Governance dispatches the same registered message handler after a passed
	// proposal; it does not possess a private key for the governance account.
	_, err := h.a.MsgServiceRouter().Handler(msg)(h.ctx(), msg)
	return err
}

func (h *sinkHarness) checkSettlementExport(t *testing.T) {
	t.Helper()
	k := h.a.Keepers.Akash.Sink
	require.NoError(t, k.CheckBalance(h.ctx()))
	open := h.checkOpenIndex(t, h.ctx())
	g := k.ExportGenesis(h.ctx())
	require.NoError(t, g.Validate())
	cache, _ := h.ctx().CacheContext()
	s := cache.KVStore(h.a.GetKey(types.StoreKey))
	before := storeEntries(s, sinkkeeper.OpenTransfersKey.Bytes())
	queue := storeEntries(s, sinkkeeper.RefundQueueKey.Bytes())
	it := storetypes.KVStorePrefixIterator(s, nil)
	var keys [][]byte
	for ; it.Valid(); it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}
	require.NoError(t, it.Close())
	for _, key := range keys {
		s.Delete(key)
	}
	require.NoError(t, k.InitGenesis(cache, g))
	require.Equal(t, g, k.ExportGenesis(cache))
	require.NoError(t, k.CheckBalance(cache))
	// InitGenesis rebuilds the derived open index and refund queue exactly.
	require.Equal(t, before, storeEntries(s, sinkkeeper.OpenTransfersKey.Bytes()))
	require.Equal(t, queue, storeEntries(s, sinkkeeper.RefundQueueKey.Bytes()))
	require.Equal(t, open, h.checkOpenIndex(t, cache))
}

func TestSinkMintReportsCloseOnce(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	p := k.GetParams(h.ctx())
	p.MaxPendingPerAccount = 1
	require.NoError(t, k.SetParams(h.ctx(), p))
	res := h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	original, _ := k.GetTransfer(h.ctx(), 1)
	for _, i := range []int{0, 0, 1} {
		res = h.report(t, i, 1, types.Outcome_OUTCOME_MINTED)
		require.Zero(t, res.Code, res.Log)
	}
	require.Len(t, h.settlement(t, 1).Votes, 2)
	require.Equal(t, uint32(1), k.GetAccountUsage(h.ctx(), h.sender).PendingCount)
	res = h.report(t, 4, 1, types.Outcome_OUTCOME_MINTED)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "not a current sink reporter")
	forged := h.reportMsg(t, 0, 1, types.Outcome_OUTCOME_MINTED)
	res = h.deliverBytes(t, h.txFrom(t, h.reporters[1], "", forged))
	require.NotZero(t, res.Code) // Cannot submit another reporter's vote.
	bad := h.reportMsg(t, 2, 1, types.Outcome_OUTCOME_MINTED)
	bad.Observation.TransactionSignature = []byte{1}
	res = h.deliverBytes(t, h.txFrom(t, h.reporters[2], "", bad))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "64-byte")
	before := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sinkkeeper.Address())
	res = h.report(t, 2, 1, types.Outcome_OUTCOME_MINTED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.Status_STATUS_MINTED, h.settlement(t, 1).Status)
	require.Equal(t, uint64(100), k.GetTotals(h.ctx()).MintedAmount)
	require.Zero(t, k.GetTotals(h.ctx()).PendingCount)
	require.Zero(t, k.GetAccountUsage(h.ctx(), h.sender).PendingCount)
	require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sinkkeeper.Address()))
	res = h.report(t, 3, 1, types.Outcome_OUTCOME_MINTED)
	require.Zero(t, res.Code, res.Log)
	require.Zero(t, k.GetTotals(h.ctx()).PendingCount)
	res = h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log) // Slot reopened.
	res = h.report(t, 3, 1, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.Hold_HOLD_CONFLICT, h.settlement(t, 1).Hold)
	require.Equal(t, types.Status_STATUS_MINTED, h.settlement(t, 1).Status)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Error(t, h.resolve(t, 1, types.Decision_DECISION_RESTART))
	// Without a void order governance cannot confirm a void, even to reopen a
	// contested mint (TestSinkGovernanceReopensContestedMint).
	require.ErrorContains(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_VOID), "void order")
	require.NoError(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_MINT))
	entry, _ := k.GetTransfer(h.ctx(), 1)
	require.Equal(t, original.Record, entry.Record)
	require.Equal(t, types.Hold_HOLD_NONE, entry.Settlement.Hold)
	h.checkSettlementExport(t)
}

func TestSinkRefundExactOnceAndBankRollback(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	original, _ := k.GetTransfer(h.ctx(), 1)
	require.NoError(t, k.RequestVoid(h.ctx(), authtypes.NewModuleAddress(govtypes.ModuleName).String(), 1))
	res = h.refund(t, 1)
	require.NotZero(t, res.Code) // An Akash order is not a Solana void.
	h.quorum(t, 1, types.Outcome_OUTCOME_VOIDED)
	s := h.settlement(t, 1)
	require.Equal(t, types.Status_STATUS_VOIDED, s.Status)
	require.Equal(t, uint64(60), k.GetTotals(h.ctx()).VoidedAmount)
	require.Zero(t, k.GetTotals(h.ctx()).PendingAmount)
	require.Equal(t, uint64(1), k.GetTotals(h.ctx()).PendingCount)
	res = h.report(t, 0, 1, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, s.ChallengeEnd, h.settlement(t, 1).ChallengeEnd)
	h.now = time.Unix(s.ChallengeEnd-2, 0)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "not finished")
	failBank := true
	h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
		if failBank && from.Equals(sinkkeeper.Address()) {
			return nil, fmt.Errorf("injected bank rejection")
		}
		return to, nil
	})
	before := h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.sender, "uakt")
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "injected bank rejection")
	require.Zero(t, k.GetTotals(h.ctx()).RefundedToday)
	require.Equal(t, types.Status_STATUS_VOIDED, h.settlement(t, 1).Status)
	require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.sender, "uakt"))
	failBank = false
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, before.Add(sdk.NewInt64Coin("uakt", 60)), h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.sender, "uakt"))
	totals := k.GetTotals(h.ctx())
	require.Equal(t, uint64(60), totals.RefundedAmount)
	require.Equal(t, uint64(60), totals.RefundedToday)
	require.Zero(t, totals.PendingCount)
	require.Zero(t, totals.VoidedAmount)
	require.Zero(t, k.GetAccountUsage(h.ctx(), h.sender).PendingCount)
	require.Equal(t, types.Status_STATUS_REFUNDED, h.settlement(t, 1).Status)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, totals, k.GetTotals(h.ctx()))
	entry, _ := k.GetTransfer(h.ctx(), 1)
	require.Equal(t, original.Record, entry.Record)
	require.Error(t, h.a.Keepers.Cosmos.Bank.SendCoinsFromModuleToAccount(h.ctx(), types.ModuleName, h.sender, sdk.NewCoins(sdk.NewInt64Coin("uakt", 1))))
	res = h.report(t, 3, 1, types.Outcome_OUTCOME_MINTED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.Hold_HOLD_CONFLICT, h.settlement(t, 1).Hold)
	require.Error(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_MINT))
	require.Error(t, h.resolve(t, 1, types.Decision_DECISION_RESTART))
	h.checkSettlementExport(t)
}

func TestSinkVetoConflictAndGovernanceResolution(t *testing.T) {
	for _, mode := range []string{"veto", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			h := reportingHarness(t)
			k := h.a.Keepers.Akash.Sink
			res := h.deliver(t, "", h.deposit(60))
			require.Zero(t, res.Code, res.Log)
			h.quorum(t, 1, types.Outcome_OUTCOME_VOIDED)
			old := h.reportMsg(t, 0, 1, types.Outcome_OUTCOME_VOIDED)
			outsider := sdk.AccAddress(h.reporters[4].PubKey().Address()).String()
			res = h.deliverBytes(t, h.txFrom(t, h.reporters[4], "", &types.MsgVetoVoid{Signer: outsider, Id: 1, Reason: "outsider"}))
			require.NotZero(t, res.Code)
			if mode == "veto" {
				member := sdk.AccAddress(h.reporters[3].PubKey().Address()).String()
				res = h.deliverBytes(t, h.txFrom(t, h.reporters[3], "", &types.MsgVetoVoid{Signer: member, Id: 1, Reason: "node does not show a finalized void"}))
				require.Zero(t, res.Code, res.Log)
				require.Equal(t, types.Hold_HOLD_VETO, h.settlement(t, 1).Hold)
			} else {
				// One reporter equivocates. Its second outcome is evidence of a
				// conflict, not another vote toward either threshold.
				res = h.report(t, 0, 1, types.Outcome_OUTCOME_MINTED)
				require.Zero(t, res.Code, res.Log)
				require.Equal(t, types.Hold_HOLD_CONFLICT, h.settlement(t, 1).Hold)
			}
			h.checkSettlementExport(t)
			h.now = time.Unix(h.settlement(t, 1).ChallengeEnd, 0)
			res = h.refund(t, 1)
			require.NotZero(t, res.Code)
			gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
			require.NoError(t, k.RequestVoid(h.ctx(), gov, 1))
			res = h.refund(t, 1)
			require.NotZero(t, res.Code)
			require.Error(t, k.ResolveTransfer(h.ctx(), &types.MsgResolveTransfer{Authority: gov, Id: 1, Decision: types.Decision_DECISION_CONFIRM_VOID, Reason: "missing evidence"}))
			res = h.deliver(t, "", &types.MsgResolveTransfer{Authority: h.sender.String(), Id: 1, Decision: types.Decision_DECISION_RESTART, Reason: "not governance"})
			require.NotZero(t, res.Code)
			require.NoError(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_VOID))
			s := h.settlement(t, 1)
			require.Equal(t, uint64(2), s.Round)
			require.Equal(t, types.Hold_HOLD_NONE, s.Hold)
			require.Greater(t, s.ChallengeEnd, h.now.Unix())
			res = h.refund(t, 1)
			require.NotZero(t, res.Code) // Governance cannot bypass the new window.
			res = h.deliverBytes(t, h.txFrom(t, h.reporters[0], "", old))
			require.NotZero(t, res.Code)
			require.Contains(t, res.Log, "stale report round")
			// Reject the void and require a completely new round of observations.
			require.NoError(t, h.resolve(t, 1, types.Decision_DECISION_RESTART))
			require.Equal(t, types.Status_STATUS_PENDING, h.settlement(t, 1).Status)
			require.Equal(t, uint64(60), k.GetTotals(h.ctx()).PendingAmount)
			require.Zero(t, k.GetTotals(h.ctx()).VoidedAmount)
			h.quorum(t, 1, types.Outcome_OUTCOME_MINTED)
			require.Equal(t, types.Status_STATUS_MINTED, h.settlement(t, 1).Status)
			h.checkSettlementExport(t)
		})
	}
}

func TestSinkReporterRotationRestartsAuthorization(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.Outcome_OUTCOME_VOIDED)
	old := h.reportMsg(t, 1, 1, types.Outcome_OUTCOME_VOIDED)
	oldDeadline := h.settlement(t, 1).ChallengeEnd
	p := k.GetParams(h.ctx())
	p.Reporters[0] = sdk.AccAddress(h.reporters[4].PubKey().Address()).String()
	require.NoError(t, k.SetParams(h.ctx(), p))
	require.Equal(t, old.ReporterEpoch+1, k.GetTotals(h.ctx()).ReporterEpoch)
	h.now = time.Unix(oldDeadline, 0)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "stale void authorization")
	h.checkSettlementExport(t) // Export/import must retain the invalidation.
	res = h.deliverBytes(t, h.txFrom(t, h.reporters[1], "", old))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "stale reporter epoch")
	res = h.report(t, 0, 1, types.Outcome_OUTCOME_VOIDED)
	require.NotZero(t, res.Code)
	res = h.report(t, 1, 1, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.Status_STATUS_PENDING, h.settlement(t, 1).Status)
	require.Equal(t, uint64(60), k.GetTotals(h.ctx()).PendingAmount)
	require.Zero(t, k.GetTotals(h.ctx()).VoidedAmount)
	for _, i := range []int{2, 4} {
		res = h.report(t, i, 1, types.Outcome_OUTCOME_VOIDED)
		require.Zero(t, res.Code, res.Log)
	}
	s := h.settlement(t, 1)
	require.Greater(t, s.ChallengeEnd, oldDeadline)
	p = k.GetParams(h.ctx())
	p.ChallengePeriodSeconds = types.DefaultChallengePeriodSeconds + 1
	p.Reporters[0], p.Reporters[1] = p.Reporters[1], p.Reporters[0]
	require.NoError(t, k.SetParams(h.ctx(), p))
	require.Equal(t, old.ReporterEpoch+1, k.GetTotals(h.ctx()).ReporterEpoch) // Reordering is not rotation.
	require.Equal(t, s.ChallengeEnd, h.settlement(t, 1).ChallengeEnd)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	h.now = time.Unix(s.ChallengeEnd-1, 0)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log) // Exactly the deadline.
	h.checkSettlementExport(t)
}

func TestSinkRotationCannotRemoveHold(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.Outcome_OUTCOME_VOIDED)
	member := sdk.AccAddress(h.reporters[3].PubKey().Address()).String()
	res = h.deliverBytes(t, h.txFrom(t, h.reporters[3], "", &types.MsgVetoVoid{Signer: member, Id: 1, Reason: "disputed void"}))
	require.Zero(t, res.Code, res.Log)
	p := k.GetParams(h.ctx())
	p.ReporterThreshold = 4
	require.NoError(t, k.SetParams(h.ctx(), p))
	for i := 0; i < 4; i++ {
		res = h.report(t, i, 1, types.Outcome_OUTCOME_VOIDED)
		require.Zero(t, res.Code, res.Log)
	}
	require.Equal(t, types.Hold_HOLD_VETO, h.settlement(t, 1).Hold)
	require.Equal(t, types.Status_STATUS_PENDING, h.settlement(t, 1).Status)
	h.now = h.now.Add(48 * time.Hour)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	h.checkSettlementExport(t)
}

func TestSinkRefundPauseAndDailyCap(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	for id := uint64(1); id <= 2; id++ {
		res := h.deliver(t, "", h.deposit(60))
		require.Zero(t, res.Code, res.Log)
		h.quorum(t, id, types.Outcome_OUTCOME_VOIDED)
	}
	h.now = time.Unix(h.settlement(t, 2).ChallengeEnd, 0)
	p := k.GetParams(h.ctx())
	p.RefundsEnabled = false
	require.NoError(t, k.SetParams(h.ctx(), p))
	res := h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "refunds disabled")
	p.RefundsEnabled = true
	require.NoError(t, k.SetParams(h.ctx(), p))
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(60), k.GetTotals(h.ctx()).RefundedToday)
	res = h.refund(t, 2)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "daily refund cap")
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	require.NoError(t, k.RequestVoid(h.ctx(), gov, 2))
	require.NoError(t, h.resolve(t, 2, types.Decision_DECISION_CONFIRM_VOID))
	require.NoError(t, k.SetParams(h.ctx(), p))
	// Neither governance nor params reset the day's refunds.
	require.Equal(t, uint64(60), k.GetTotals(h.ctx()).RefundedToday)
	res = h.refund(t, 2)
	require.NotZero(t, res.Code) // the new window has not passed
	oldDay := k.GetTotals(h.ctx()).RefundDay
	h.now = time.Unix(h.settlement(t, 2).ChallengeEnd, 0)
	res = h.refund(t, 2)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(120), k.GetTotals(h.ctx()).RefundedAmount)
	require.Equal(t, uint64(60), k.GetTotals(h.ctx()).RefundedToday)
	require.Greater(t, k.GetTotals(h.ctx()).RefundDay, oldDay)
	h.checkSettlementExport(t)
}

func TestSinkGovernanceWhenReportersUnavailable(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	p := k.GetParams(h.ctx())
	p.Enabled = false
	p.Reporters = nil
	p.ReporterThreshold = 0
	require.NoError(t, k.SetParams(h.ctx(), p))
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	require.NoError(t, k.RequestVoid(h.ctx(), gov, 1))
	require.NoError(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_VOID))
	h.now = time.Unix(h.settlement(t, 1).ChallengeEnd, 0)
	require.NoError(t, k.VetoVoid(h.ctx(), &types.MsgVetoVoid{Signer: gov, Id: 1, Reason: "stop before payment"}))
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.NoError(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_VOID))
	h.now = time.Unix(h.settlement(t, 1).ChallengeEnd-1, 0)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	h.checkSettlementExport(t)
}
