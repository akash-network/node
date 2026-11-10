package keeper

import (
	"testing"
	"time"

	sdkerrors "cosmossdk.io/errors"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestSettlementPolicyAndQueueRecovery(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	p := g.Params
	p.RefundsEnabled = true
	require.NoError(t, k.SetParams(ctx, p))

	current, err := k.GetCurrentPolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(2), current.Version)
	head, ok, err := k.refundHead(ctx, current.Version)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, g.Records[6].ID, head)
	stale := g.Records[5].ID
	require.Error(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{
		Executor: testOwner(1).String(),
		ID:       stale,
	}))

	before, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	events := ctx.EventManager().Events()
	bank.fail = true
	require.Error(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{
		Executor: testOwner(1).String(),
		ID:       head,
	}))

	after, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, events, ctx.EventManager().Events())
	bank.fail = false
	require.NoError(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{
		Executor: testOwner(1).String(),
		ID:       head,
	}))

	owner, err := sdk.AccAddressFromBech32(head.Owner)
	require.NoError(t, err)
	require.Equal(t, sdk.NewCoins(testCoin(10)), bank.GetAllBalances(ctx, owner))
	require.NoError(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{
		Executor: testOwner(1).String(),
		ID:       head,
	}))

	require.Equal(t, sdk.NewCoins(testCoin(10)), bank.GetAllBalances(ctx, owner))
	head, ok, err = k.refundHead(ctx, current.Version)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, g.Records[7].ID, head)
	p.Reporters = nil
	require.NoError(t, k.SetParams(ctx, p))

	current, err = k.GetCurrentPolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(3), current.Version)
	authorized, err := k.GetTransfer(ctx, head)
	require.NoError(t, err)
	require.Equal(t, g.Records[7].Settlement, authorized.Settlement)
	require.NoError(t, k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{
		Executor: testOwner(1).String(),
		ID:       head,
	}))

	refunded, err := k.GetTransfer(ctx, head)
	require.NoError(t, err)
	require.Equal(t, types.StatusRefunded, refunded.Status)
	require.Equal(t, authorized.Settlement.GovernanceObservation, refunded.Settlement.GovernanceObservation)
	require.Equal(t, authorized.Settlement.VoidAuthorization, refunded.Settlement.VoidAuthorization)
	owner, err = sdk.AccAddressFromBech32(head.Owner)
	require.NoError(t, err)
	require.Equal(t, sdk.NewCoins(testCoin(10)), bank.GetAllBalances(ctx, owner))
	terminal := g.Records[11].ID
	require.NoError(t, k.Resolve(ctx, &types.MsgConfirmVoid{
		Authority:     k.authority,
		ID:            terminal,
		ExpectedRound: 1,
		Reason:        "resolve paid conflict",
		Evidence:      testEvidence(),
	}))

	paid, err := k.GetTransfer(ctx, terminal)
	require.NoError(t, err)
	require.Equal(t, types.StatusRefunded, paid.Status)
	require.Nil(t, paid.Settlement.VoidAuthorization)
	require.Equal(t, types.HoldNone, paid.Settlement.Hold)
	require.ErrorIs(t, k.Resolve(ctx, &types.MsgConfirmVoid{
		Authority:     k.authority,
		ID:            terminal,
		ExpectedRound: 1,
		Reason:        "stale",
		Evidence:      testEvidence(),
	}), types.ErrStaleReport)

	require.Error(t, k.Resolve(ctx, &types.MsgConfirmMint{
		Authority:     k.authority,
		ID:            terminal,
		ExpectedRound: 2,
		Reason:        "reverse payment",
		Evidence:      testEvidence(),
	}))

	_, err = k.ExportGenesis(ctx)
	require.NoError(t, err)
}

func TestFinalizeRefundAuthorizationErrors(t *testing.T) {
	for _, tc := range []struct {
		name          string
		index         int
		rotate        bool
		missingQuorum bool
		unknownHold   bool
		want          *sdkerrors.Error
		code          uint32
	}{
		{
			name:  "stale policy",
			index: 5,
			want:  types.ErrStaleReport,
			code:  24,
		},
		{
			name:  "veto",
			index: 8,
			want:  types.ErrSettlementHeld,
			code:  38,
		},
		{
			name:  "conflict",
			index: 9,
			want:  types.ErrSettlementHeld,
			code:  38,
		},
		{
			name:   "veto after rotation",
			index:  8,
			rotate: true,
			want:   types.ErrSettlementHeld,
			code:   38,
		},
		{
			name:   "conflict after rotation",
			index:  9,
			rotate: true,
			want:   types.ErrSettlementHeld,
			code:   38,
		},
		{
			name:  "pending without authorization",
			index: 0,
			want:  types.ErrInvalidSettlement,
			code:  7,
		},
		{
			name:  "minted conflict",
			index: 4,
			want:  types.ErrInvalidSettlement,
			code:  7,
		},
		{
			name:          "missing stale quorum",
			index:         5,
			missingQuorum: true,
			want:          types.ErrInvalidSettlement,
			code:          7,
		},
		{
			name:          "missing held quorum",
			index:         8,
			missingQuorum: true,
			want:          types.ErrInvalidSettlement,
			code:          7,
		},
		{
			name:          "missing stale held quorum",
			index:         9,
			rotate:        true,
			missingQuorum: true,
			want:          types.ErrInvalidSettlement,
			code:          7,
		},
		{
			name:        "unknown hold",
			index:       5,
			unknownHold: true,
			want:        types.ErrInvalidSettlement,
			code:        7,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			g.Params.RefundsEnabled = true
			importTestGenesis(t, k, ctx, bank, g)

			if tc.rotate {
				p := g.Params
				p.Reporters = nil
				require.NoError(t, k.SetParams(ctx, p))
			}

			r := g.Records[tc.index]
			if tc.missingQuorum || tc.unknownHold {
				if tc.missingQuorum {
					r.Settlement.Votes = nil
				} else {
					r.Settlement.Hold = types.Hold(99)
				}

				require.NoError(t, k.settlements.Set(ctx, types.NewSettlementID(r.Status, r.ID), r.Settlement))
			}

			snapshot := func() map[string]string {
				state := make(map[string]string)
				for _, key := range []*storetypes.KVStoreKey{k.skey, bank.key} {
					it := ctx.KVStore(key).Iterator(nil, nil)
					for ; it.Valid(); it.Next() {
						state[key.Name()+"/"+string(it.Key())] = string(it.Value())
					}

					require.NoError(t, it.Close())
				}

				return state
			}

			before := snapshot()
			events := ctx.EventManager().Events()
			err := k.FinalizeRefund(ctx, &types.MsgFinalizeRefund{
				Executor: testOwner(1).String(),
				ID:       r.ID,
			})
			require.Equal(t, before, snapshot())
			require.Equal(t, events, ctx.EventManager().Events())
			require.ErrorIs(t, err, tc.want)

			codespace, code, _ := sdkerrors.ABCIInfo(err, false)
			require.Equal(t, types.ModuleName, codespace)
			require.Equal(t, tc.code, code)

			grpcCode := codes.FailedPrecondition
			if tc.want == types.ErrInvalidSettlement {
				grpcCode = codes.InvalidArgument
			}

			require.Equal(t, grpcCode, status.Code(err))
		})
	}
}

func TestReportsRetainEvidenceAndVetoAcrossPolicies(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	terminal := g.Records[3]
	msg := &types.MsgReportOutcome{
		Reporter:      g.Policies[1].Members[3],
		ID:            terminal.ID,
		PolicyVersion: 2,
		Round:         1,
		Observation: types.Observation{
			Outcome:              types.OutcomeVoided,
			Slot:                 testEvidence().Slot,
			TransactionSignature: testEvidence().TransactionSignature,
		},
	}

	require.NoError(t, k.ReportOutcome(ctx, msg))
	got, err := k.GetTransfer(ctx, terminal.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusMinted, got.Status)
	require.Equal(t, uint64(1), got.Settlement.Policy.Version)
	require.Equal(t, types.HoldConflict, got.Settlement.Hold)
	require.Equal(t, terminal.Settlement.Votes, got.Settlement.Votes[:3])
	msg.Observation.Slot++
	require.NoError(t, k.ReportOutcome(ctx, msg))
	again, err := k.GetTransfer(ctx, terminal.ID)
	require.NoError(t, err)
	require.Equal(t, got, again)
	require.NoError(t, k.RequestVoid(ctx, &types.MsgRequestVoid{
		Authority:     k.authority,
		ID:            terminal.ID,
		ExpectedRound: 1,
	}))

	require.NoError(t, k.Resolve(ctx, &types.MsgConfirmVoid{
		Authority:     k.authority,
		ID:            terminal.ID,
		ExpectedRound: 1,
		Reason:        "contested mint",
		Evidence:      testEvidence(),
	}))

	got, err = k.GetTransfer(ctx, terminal.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusRefundPending, got.Status)
	require.Empty(t, got.Settlement.Votes)
	require.NotNil(t, got.Settlement.GovernanceObservation)
	p := g.Params
	p.Reporters = []string{testOwner(1).String(), testOwner(2).String(), testOwner(3).String(), testOwner(4).String()}
	require.NoError(t, k.SetParams(ctx, p))

	veto := g.Records[8]
	msg = &types.MsgReportOutcome{
		Reporter:      testOwner(1).String(),
		ID:            veto.ID,
		PolicyVersion: 3,
		Round:         1,
		Observation: types.Observation{
			Outcome:              types.OutcomeVoided,
			Slot:                 testEvidence().Slot,
			TransactionSignature: testEvidence().TransactionSignature,
		},
	}

	require.NoError(t, k.ReportOutcome(ctx, msg))
	got, err = k.GetTransfer(ctx, veto.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusRefundPending, got.Status)
	require.Equal(t, types.HoldVeto, got.Settlement.Hold)
	require.Equal(t, veto.Settlement.VetoBy, got.Settlement.VetoBy)
	require.Equal(t, veto.Settlement.VetoPolicyVersion, got.Settlement.VetoPolicyVersion)
	require.Len(t, got.Settlement.Votes, 3)
	require.Equal(t, veto.Settlement.ChallengeEnd, got.Settlement.ChallengeEnd)
	msg.PolicyVersion = 2
	require.Error(t, k.ReportOutcome(ctx, msg))
	_, err = k.ExportGenesis(ctx)
	require.NoError(t, err)
}

func TestVetoDeadlineAndGovernanceRoundBoundaries(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	id := g.Records[6].ID
	msg := &types.MsgVetoVoid{
		Signer: g.Policies[1].Members[0],
		ID:     id,
		Reason: "review",
	}

	require.ErrorIs(t, k.VetoVoid(ctx, msg), types.ErrChallengeWindowEnded)
	msg.Signer = k.authority
	require.NoError(t, k.VetoVoid(ctx, msg))
	got, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.HoldVeto, got.Settlement.Hold)
	require.NoError(t, k.Resolve(ctx, &types.MsgRestartTransfer{
		Authority:     k.authority,
		ID:            id,
		ExpectedRound: 1,
		Reason:        "restart",
	}))

	got, err = k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), got.Settlement.Round)
	require.Empty(t, got.Settlement.VetoBy)
	require.Zero(t, got.Settlement.ChallengeEnd)
	require.Empty(t, got.Settlement.Votes)
	before, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Error(t, k.Resolve(ctx, &types.MsgConfirmVoid{
		Authority:     k.authority,
		ID:            id,
		ExpectedRound: 2,
		Reason:        "missing order",
		Evidence:      testEvidence(),
	}))

	after, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, k.RequestVoid(ctx, &types.MsgRequestVoid{
		Authority:     k.authority,
		ID:            id,
		ExpectedRound: 2,
	}))

	require.NoError(t, k.Resolve(ctx, &types.MsgConfirmVoid{
		Authority:     k.authority,
		ID:            id,
		ExpectedRound: 2,
		Reason:        "confirmed",
		Evidence:      testEvidence(),
	}))

	got, err = k.GetTransfer(ctx, id)
	require.NoError(t, err)
	deadline := ctx.BlockTime().Add(g.Params.ReporterChallengePeriod)
	require.Equal(t, deadline.Unix(), got.Settlement.ChallengeEnd)
	p := g.Params
	p.ReporterChallengePeriod += time.Hour
	require.NoError(t, k.SetParams(ctx, p))

	got, err = k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, deadline.Unix(), got.Settlement.ChallengeEnd)
}

func TestUnheldAttemptRefreshClearsStaleVotes(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	g.Records[0].Settlement.Policy = g.Policies[0]
	g.Records[0].Settlement.Votes = quorum(g.Policies[0], types.OutcomeVoided)[:1]
	importTestGenesis(t, k, ctx, bank, g)
	vote := quorum(g.Policies[1], types.OutcomeMinted)[0]
	msg := &types.MsgReportOutcome{
		Reporter:      vote.Reporter,
		ID:            g.Records[0].ID,
		PolicyVersion: 1,
		Round:         1,
		Observation:   vote.Observation,
	}
	before, err := k.GetTransfer(ctx, msg.ID)
	require.NoError(t, err)
	require.ErrorIs(t, k.ReportOutcome(ctx, msg), types.ErrStaleReport)
	unchanged, err := k.GetTransfer(ctx, msg.ID)
	require.NoError(t, err)
	require.Equal(t, before, unchanged)
	msg.PolicyVersion = 2
	require.NoError(t, k.ReportOutcome(ctx, msg))
	got, err := k.GetTransfer(ctx, msg.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatusTransferPending, got.Status)
	require.Equal(t, uint64(1), got.Settlement.Round)
	require.Equal(t, g.Policies[1], got.Settlement.Policy)
	require.Equal(t, []types.Vote{vote}, got.Settlement.Votes)
	_, err = k.ExportGenesis(ctx)
	require.NoError(t, err)
}
