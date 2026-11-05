package handler_test

import (
	"bytes"
	"testing"
	"time"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/x/sink/keeper"
)

func TestSinkMintReportsCloseOnce(t *testing.T) {
	h := setupTestSuite(t)
	id := h.deposit(60)
	before := h.BankKeeper().GetAllBalances(h.ctx, keeper.Address())
	h.quorum(id, types.OutcomeMinted)
	s := h.transfer(id)
	require.Equal(t, types.StatusMinted, s.Status)
	require.Len(t, s.Settlement.Votes, 3)
	msg := h.reportMsg(0, id, types.OutcomeMinted)
	msg.Observation.Slot++
	_, err := h.server.ReportOutcome(h.ctx, msg)
	require.NoError(t, err)
	require.Equal(t, s, h.transfer(id))
	require.Equal(t, coin(60), h.totals().MintedAmount)
	require.Equal(t, uint64(1), h.totals().MintedCount)
	require.Zero(t, h.totals().PendingCount)
	require.Error(t, h.refund(id))
	require.Equal(t, before, h.BankKeeper().GetAllBalances(h.ctx, keeper.Address()))
	h.checkSettlementExport()
}

func TestSinkVetoConflictAndGovernanceResolution(t *testing.T) {
	h := setupTestSuite(t)
	id := h.deposit(60)
	h.quorum(id, types.OutcomeVoided)
	_, err := h.server.VetoVoid(h.ctx, &types.MsgVetoVoid{
		Signer: h.reporters[0],
		ID:     id,
		Reason: "check evidence",
	})
	require.NoError(t, err)
	require.Equal(t, types.HoldVeto, h.transfer(id).Settlement.Hold)
	require.NoError(t, h.report(0, id, types.OutcomeMinted))
	require.Equal(t, types.HoldConflict, h.transfer(id).Settlement.Hold)
	round := h.transfer(id).Settlement.Round
	require.NoError(t, h.resolve(id, types.DecisionRestart))
	require.Equal(t, round+1, h.transfer(id).Settlement.Round)
	require.Empty(t, h.transfer(id).Settlement.Votes)
	require.Empty(t, h.transfer(id).Settlement.VetoBy)
	stale := &types.MsgConfirmMint{
		Authority:     authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		ID:            id,
		ExpectedRound: round,
		Reason:        "stale proposal",
		Evidence: types.SolanaEvidence{
			Slot:                 100,
			TransactionSignature: bytes.Repeat([]byte{1}, 64),
		},
	}
	_, err = h.server.ConfirmMint(h.ctx, stale)
	require.ErrorIs(t, err, types.ErrStaleReport)
	require.Error(t, h.resolve(id, types.DecisionConfirmVoid))
	require.NoError(t, h.order(id))
	require.NoError(t, h.resolve(id, types.DecisionConfirmVoid))
	h.mature(id)
	require.NoError(t, h.refund(id))
	require.Error(t, h.resolve(id, types.DecisionConfirmMint))
	require.Error(t, h.resolve(id, types.DecisionRestart))
	h.checkSettlementExport()
}

func TestSinkReporterRotationRestartsAuthorization(t *testing.T) {
	h := setupTestSuite(t)
	id := h.deposit(60)
	h.quorum(id, types.OutcomeVoided)
	old := h.transfer(id)
	h.rotate()
	state, err := h.SinkKeeper().NewQuerier().State(h.ctx, nil)
	require.NoError(t, err)
	require.Nil(t, state.RefundHead)
	h.ctx = h.ctx.WithBlockTime(time.Unix(old.Settlement.ChallengeEnd, 0).UTC())
	require.Error(t, h.refund(id))
	require.NoError(t, h.report(1, id, types.OutcomeVoided))
	s := h.transfer(id)
	require.Equal(t, types.StatusTransferPending, s.Status)
	require.Len(t, s.Settlement.Votes, 1)
	require.Zero(t, s.Settlement.ChallengeEnd)

	for _, r := range []int{2, 3} {
		require.NoError(t, h.report(r, id, types.OutcomeVoided))
	}

	s = h.transfer(id)
	require.Equal(t, types.StatusRefundPending, s.Status)
	require.Greater(t, s.Settlement.ChallengeEnd, old.Settlement.ChallengeEnd)
	h.checkSettlementExport()
}

func TestSinkRotationCannotRemoveHold(t *testing.T) {
	h := setupTestSuite(t)
	id := h.deposit(60)
	h.quorum(id, types.OutcomeVoided)
	_, err := h.server.VetoVoid(h.ctx, &types.MsgVetoVoid{
		Signer: h.reporters[0],
		ID:     id,
		Reason: "keep me",
	})
	require.NoError(t, err)
	h.rotate()
	require.NoError(t, h.report(1, id, types.OutcomeVoided))
	s := h.transfer(id)
	require.Equal(t, types.HoldVeto, s.Settlement.Hold)
	require.Equal(t, "keep me", s.Settlement.VetoReason)
	require.Positive(t, s.Settlement.ChallengeEnd)
	require.Equal(t, types.StatusRefundPending, s.Status)
	h.checkSettlementExport()
}

func TestSinkTerminalEvidenceSurvivesRotation(t *testing.T) {
	for _, outcome := range []types.Outcome{types.OutcomeMinted, types.OutcomeVoided} {
		t.Run(outcome.String(), func(t *testing.T) {
			h := setupTestSuite(t)
			id := h.deposit(60)
			h.quorum(id, outcome)

			if outcome == types.OutcomeVoided {
				h.mature(id)
				require.NoError(t, h.refund(id))
			}

			before := h.transfer(id)
			h.rotate()
			opposite := types.OutcomeVoided
			if outcome == types.OutcomeVoided {
				opposite = types.OutcomeMinted
			}

			require.NoError(t, h.report(4, id, opposite))
			after := h.transfer(id)
			require.Equal(t, before.Status, after.Status)
			require.Equal(t, before.Settlement.Policy.Version, after.Settlement.Policy.Version)
			require.Equal(t, before.Settlement.Votes, after.Settlement.Votes[:len(before.Settlement.Votes)])
			require.Equal(t, types.HoldConflict, after.Settlement.Hold)
			t.Run("disputes query", func(t *testing.T) {
				disputes, err := h.SinkKeeper().NewQuerier().Disputes(h.ctx, nil)
				require.NoError(t, err)
				require.Len(t, disputes.Transfers, 1)
			})

			if outcome == types.OutcomeVoided {
				require.NoError(t, h.resolve(id, types.DecisionConfirmVoid))
				require.Equal(t, types.StatusRefunded, h.transfer(id).Status)
			}

			h.checkSettlementExport()
		})
	}
}

func TestSinkGovernanceWhenReportersUnavailable(t *testing.T) {
	h := setupTestSuite(t)
	id := h.deposit(60)
	require.NoError(t, h.order(id))
	require.NoError(t, h.resolve(id, types.DecisionConfirmVoid))
	version := h.transfer(id).Settlement.Policy.Version
	p := h.params()
	p.Enabled = false
	p.Reporters = nil
	require.NoError(t, h.update(p))
	require.Equal(t, version, h.transfer(id).Settlement.Policy.Version)
	h.mature(id)
	require.NoError(t, h.refund(id))
	h.checkSettlementExport()
}

func TestSinkGovernanceReopensContestedMint(t *testing.T) {
	h := setupTestSuite(t)
	id := h.deposit(100)
	h.quorum(id, types.OutcomeMinted)
	require.Error(t, h.order(id))
	require.Error(t, h.resolve(id, types.DecisionConfirmVoid))
	require.NoError(t, h.report(3, id, types.OutcomeVoided))
	p := h.params()
	p.MaxAmountPerTransfer = coin(50)
	p.MaxEpochRefund = coin(50)
	require.NoError(t, h.update(p))
	require.NoError(t, h.order(id))
	require.Error(t, h.resolve(id, types.DecisionConfirmVoid))
	p.MaxEpochRefund = coin(100)
	require.NoError(t, h.update(p))
	require.NoError(t, h.resolve(id, types.DecisionConfirmVoid))
	require.True(t, h.totals().MintedAmount.IsZero())
	require.Zero(t, h.totals().MintedCount)
	require.Equal(t, coin(100), h.totals().RefundPendingAmount)
	h.mature(id)
	require.NoError(t, h.refund(id))
	h.checkSettlementExport()
}

func TestSinkParameterFloors(t *testing.T) {
	h := setupTestSuite(t)
	id := h.deposit(100)
	p := h.params()
	p.MaxAmountPerTransfer = coin(50)
	p.MaxEpochRefund = coin(50)
	require.Error(t, h.update(p))
	p = h.params()
	p.ReporterChallengePeriod = time.Duration(types.MinChallengePeriodSeconds)*time.Second - time.Nanosecond
	require.Error(t, h.update(p))
	h.quorum(id, types.OutcomeVoided)
	deadline := h.transfer(id).Settlement.ChallengeEnd
	p = h.params()
	p.ReporterChallengePeriod += time.Hour
	require.NoError(t, h.update(p))
	require.Equal(t, deadline, h.transfer(id).Settlement.ChallengeEnd)
}
