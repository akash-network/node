package keeper

import (
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestSettlementPolicyAndQueueRecovery(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	p := g.Params
	p.RefundsEnabled = true
	require.NoError(t, k.UpdateParams(ctx, &types.MsgUpdateParams{Authority: k.authority, Params: p}))
	current, err := k.currentPolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(2), current.Version)
	head, ok, err := k.refundHead(ctx, current.Version)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, g.Transfers[6].ID, head)
	stale := g.Transfers[5].ID
	require.Error(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{Executor: testOwner(1).String(), ID: stale}))
	before, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	events := ctx.EventManager().Events()
	bank.fail = true
	require.Error(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{Executor: testOwner(1).String(), ID: head}))
	after, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, events, ctx.EventManager().Events())
	bank.fail = false
	require.NoError(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{Executor: testOwner(1).String(), ID: head}))
	owner, err := sdk.AccAddressFromBech32(head.Owner)
	require.NoError(t, err)
	require.Equal(t, sdk.NewCoins(testCoin(10)), bank.GetAllBalances(ctx, owner))
	require.NoError(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{Executor: testOwner(1).String(), ID: head}))
	require.Equal(t, sdk.NewCoins(testCoin(10)), bank.GetAllBalances(ctx, owner))
	head, ok, err = k.refundHead(ctx, current.Version)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, g.Transfers[7].ID, head)
	p.Reporters = nil
	require.NoError(t, k.UpdateParams(ctx, &types.MsgUpdateParams{Authority: k.authority, Params: p}))
	require.NoError(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{Executor: testOwner(1).String(), ID: head}))
	terminal := g.Transfers[11].ID
	require.NoError(t, k.Resolve(ctx, &types.MsgConfirmVoid{Authority: k.authority, ID: terminal, ExpectedRound: 1, Reason: "resolve paid conflict", Evidence: testEvidence()}))
	paid, err := k.GetTransfer(ctx, terminal)
	require.NoError(t, err)
	require.Equal(t, types.StatusRefunded, paid.Settlement.Status)
	require.False(t, paid.VoidOrdered)
	require.Equal(t, types.HoldNone, paid.Settlement.Hold)
	require.ErrorIs(t, k.Resolve(ctx, &types.MsgConfirmVoid{Authority: k.authority, ID: terminal, ExpectedRound: 1, Reason: "stale", Evidence: testEvidence()}), types.ErrStaleReport)
	require.Error(t, k.Resolve(ctx, &types.MsgConfirmMint{Authority: k.authority, ID: terminal, ExpectedRound: 2, Reason: "reverse payment", Evidence: testEvidence()}))
	_, err = k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.NoError(t, k.CheckBalance(ctx))
}

func TestReportsRetainEvidenceAndVetoAcrossPolicies(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	terminal := g.Transfers[3]
	msg := &types.MsgReportOutcome{Reporter: g.Policies[1].Members[3], ID: terminal.ID, PolicyVersion: 2, Round: 1, Observation: types.Observation{Outcome: types.OutcomeVoided, Evidence: testEvidence()}}
	require.NoError(t, k.ReportOutcome(ctx, msg))
	got, err := k.GetTransfer(ctx, terminal.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusMinted, got.Settlement.Status)
	require.Equal(t, uint64(1), got.Settlement.PolicyVersion)
	require.Equal(t, types.HoldConflict, got.Settlement.Hold)
	require.Equal(t, terminal.Settlement.Votes, got.Settlement.Votes[:3])
	msg.Observation.Evidence.Slot++
	require.NoError(t, k.ReportOutcome(ctx, msg))
	again, err := k.GetTransfer(ctx, terminal.ID)
	require.NoError(t, err)
	require.Equal(t, got, again)
	require.NoError(t, k.RequestVoid(ctx, &types.MsgRequestVoid{Authority: k.authority, ID: terminal.ID}))
	require.NoError(t, k.Resolve(ctx, &types.MsgConfirmVoid{Authority: k.authority, ID: terminal.ID, ExpectedRound: 1, Reason: "contested mint", Evidence: testEvidence()}))
	got, err = k.GetTransfer(ctx, terminal.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusRefundPending, got.Settlement.Status)
	require.Empty(t, got.Settlement.Votes)
	require.NotNil(t, got.Settlement.GovernanceEvidence)
	p := g.Params
	p.Reporters = []string{testOwner(1).String(), testOwner(2).String(), testOwner(3).String(), testOwner(4).String()}
	require.NoError(t, k.UpdateParams(ctx, &types.MsgUpdateParams{Authority: k.authority, Params: p}))
	veto := g.Transfers[8]
	msg = &types.MsgReportOutcome{Reporter: testOwner(1).String(), ID: veto.ID, PolicyVersion: 3, Round: 1, Observation: types.Observation{Outcome: types.OutcomeVoided, Evidence: testEvidence()}}
	require.NoError(t, k.ReportOutcome(ctx, msg))
	got, err = k.GetTransfer(ctx, veto.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusTransferPending, got.Settlement.Status)
	require.Equal(t, types.HoldVeto, got.Settlement.Hold)
	require.Equal(t, veto.Settlement.Veto, got.Settlement.Veto)
	require.Len(t, got.Settlement.Votes, 4)
	require.Nil(t, got.Settlement.ChallengeEnd)
	msg.PolicyVersion = 2
	require.Error(t, k.ReportOutcome(ctx, msg))
	_, err = k.ExportGenesis(ctx)
	require.NoError(t, err)
}

func TestVetoDeadlineAndGovernanceRoundBoundaries(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	id := g.Transfers[6].ID
	msg := &types.MsgVetoVoid{Signer: g.Policies[1].Members[0], ID: id, Reason: "review"}
	require.ErrorIs(t, k.VetoVoid(ctx, msg), types.ErrChallengeWindowEnded)
	msg.Signer = k.authority
	require.NoError(t, k.VetoVoid(ctx, msg))
	got, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.HoldVeto, got.Settlement.Hold)
	require.NoError(t, k.Resolve(ctx, &types.MsgRestartTransfer{Authority: k.authority, ID: id, ExpectedRound: 1, Reason: "restart"}))
	got, err = k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), got.Settlement.Round)
	require.Nil(t, got.Settlement.Veto)
	require.Nil(t, got.Settlement.ChallengeEnd)
	require.Empty(t, got.Settlement.Votes)
	before, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Error(t, k.Resolve(ctx, &types.MsgConfirmVoid{Authority: k.authority, ID: id, ExpectedRound: 2, Reason: "missing order", Evidence: testEvidence()}))
	after, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, k.RequestVoid(ctx, &types.MsgRequestVoid{Authority: k.authority, ID: id}))
	require.NoError(t, k.Resolve(ctx, &types.MsgConfirmVoid{Authority: k.authority, ID: id, ExpectedRound: 2, Reason: "confirmed", Evidence: testEvidence()}))
	got, err = k.GetTransfer(ctx, id)
	require.NoError(t, err)
	deadline := ctx.BlockTime().Add(g.Params.ReporterChallengePeriod)
	require.True(t, deadline.Equal(*got.Settlement.ChallengeEnd))
	p := g.Params
	p.ReporterChallengePeriod += time.Hour
	require.NoError(t, k.UpdateParams(ctx, &types.MsgUpdateParams{Authority: k.authority, Params: p}))
	got, err = k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.True(t, deadline.Equal(*got.Settlement.ChallengeEnd))
}

func TestRecoveredVotesRefreshWithoutReplacingEvidence(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	g.Transfers[0].Settlement.PolicyVersion = 1
	g.Transfers[0].Settlement.Votes = quorum(g.Policies[1], types.OutcomeMinted)
	importTestGenesis(t, k, ctx, bank, g)
	vote := g.Transfers[0].Settlement.Votes[0]
	evidence := vote.Observation
	evidence.Evidence.Slot++
	require.NoError(t, k.ReportOutcome(ctx, &types.MsgReportOutcome{Reporter: vote.Reporter, ID: g.Transfers[0].ID, PolicyVersion: 2, Round: 1, Observation: evidence}))
	got, err := k.GetTransfer(ctx, g.Transfers[0].ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusMinted, got.Settlement.Status)
	require.Equal(t, uint64(2), got.Settlement.PolicyVersion)
	require.Equal(t, g.Transfers[0].Settlement.Votes, got.Settlement.Votes)
	_, err = k.ExportGenesis(ctx)
	require.NoError(t, err)
}
