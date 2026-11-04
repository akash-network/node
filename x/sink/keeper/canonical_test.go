package keeper

import (
	"bytes"
	"errors"
	"math"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestCanonicalLookupRejectsZeroMultipleAndMalformedValues(t *testing.T) {
	for _, kind := range []string{"zero", "multiple", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			importTestGenesis(t, k, ctx, bank, g)
			record := g.Records[0]
			key := *types.NewSettlementID(record.Status, record.ID)
			switch kind {
			case "zero":
				require.NoError(t, k.settlements.Remove(ctx, key))
			case "multiple":
				require.NoError(t, k.settlements.Set(ctx, *types.NewSettlementID(types.StatusVoided, record.ID), record.Settlement))
			case "malformed":
				raw, err := SettlementKey(key)
				require.NoError(t, err)
				ctx.KVStore(k.skey).Set(raw, []byte{0xff})
			}

			_, err := k.GetTransfer(ctx, record.ID)
			require.Error(t, err)
			require.NotErrorIs(t, err, types.ErrTransferNotFound)
			_, err = k.ExportGenesis(ctx)
			require.Error(t, err)
		})
	}
}

func TestCanonicalKeyMoveRollsBackAfterAccountingFailure(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	before, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	events := ctx.EventManager().Events()
	validEpochs := k.epochs
	failure := errors.New("native epoch unavailable during totals event")
	k.epochs = testEpochs{err: failure}
	err = k.RequestVoid(ctx, &types.MsgRequestVoid{
		Authority:     k.authority,
		ID:            g.Records[0].ID,
		ExpectedRound: 1,
	})
	require.ErrorIs(t, err, failure)
	k.epochs = validEpochs
	after, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, events, ctx.EventManager().Events())
	oldKey, _ := SettlementKey(*types.NewSettlementID(types.StatusTransferPending, g.Records[0].ID))
	newKey, _ := SettlementKey(*types.NewSettlementID(types.StatusVoided, g.Records[0].ID))
	require.True(t, ctx.KVStore(k.skey).Has(oldKey))
	require.False(t, ctx.KVStore(k.skey).Has(newKey))
	require.NoError(t, k.CheckBalance(ctx))
}

func TestExportRejectsMalformedCanonicalKeys(t *testing.T) {
	for _, kind := range []string{"unknown status", "short ID", "trailing ID", "zero owner", "zero height", "zero sequence"} {
		t.Run(kind, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			importTestGenesis(t, k, ctx, bank, g)
			key, err := SettlementKey(*types.NewSettlementID(types.StatusTransferPending, g.Records[0].ID))
			require.NoError(t, err)
			value := bytes.Clone(ctx.KVStore(k.skey).Get(key))

			switch kind {
			case "unknown status":
				key[2] = 6
			case "short ID":
				key = key[:len(key)-1]
			case "trailing ID":
				key = append(key, 0)
			case "zero owner":
				clear(key[4:24])
			case "zero height":
				clear(key[24:32])
			case "zero sequence":
				clear(key[32:40])
			}

			ctx.KVStore(k.skey).Set(key, value)
			_, err = k.ExportGenesis(ctx)
			require.Error(t, err)
		})
	}
}

func TestGovernanceVetoImportAuthenticatesConfiguredAuthority(t *testing.T) {
	for _, authority := range []bool{false, true} {
		k, ctx, bank := testStore(t)
		g := populatedGenesis()
		s := &g.Records[8].Settlement
		s.VetoPolicyVersion = 0
		if authority {
			s.VetoBy = k.authority
		}

		require.NoError(t, g.Validate(), "SDK validates the shape, keeper authenticates governance")
		escrow, err := g.Totals.EscrowAmount()
		require.NoError(t, err)
		bank.set(ctx, Address(), sdk.NewCoins(escrow))
		err = k.InitGenesis(ctx, g)
		if authority {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "governance veto signer")
			initialized, err := k.initialized(ctx)
			require.NoError(t, err)
			require.False(t, initialized)
		}
	}
}

func TestChallengeDeadlineCeilingAndRange(t *testing.T) {
	_, ctx, _ := testStore(t)
	for _, tc := range []struct {
		name     string
		nanos    int64
		duration time.Duration
		want     int64
	}{
		{"whole seconds", 0, 2 * time.Second, 102},
		{"fractional clock", 1, 2 * time.Second, 103},
		{"fractional duration", 0, 2*time.Second + 1, 103},
		{"fractional carry", 800_000_000, 2*time.Second + 800*time.Millisecond, 104},
		{"exact fractional sum", 200_000_000, 2*time.Second + 800*time.Millisecond, 103},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := types.DefaultParams()
			p.ReporterChallengePeriod = tc.duration
			got, err := challengeDeadline(ctx.WithBlockTime(time.Unix(100, tc.nanos)), p)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	p := types.DefaultParams()
	for _, seconds := range []int64{-1, math.MaxInt64, 253402300799} {
		_, err := challengeDeadline(ctx.WithBlockTime(time.Unix(seconds, 0)), p)
		require.Error(t, err)
	}
}

func TestPermanentOrderSurvivesMintConflictRotationRestartAndRefund(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := historyGenesis(1, settlementIn(types.StatusTransferPending), 10, 0)
	g.Params.RefundsEnabled = true
	importTestGenesis(t, k, ctx, bank, g)
	id := g.Records[0].ID
	request := &types.MsgRequestVoid{
		Authority:     k.authority,
		ID:            id,
		ExpectedRound: 2,
	}

	before, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.ErrorIs(t, k.RequestVoid(ctx, request), types.ErrStaleReport)
	after, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	request.ExpectedRound = 1
	require.NoError(t, k.RequestVoid(ctx, request))
	ordered, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.StatusVoided, ordered.Status)
	receipt := *ordered.Settlement.VoidAuthorization
	totals, err := k.GetTotals(ctx)
	require.NoError(t, err)
	require.Zero(t, totals.PendingCount)
	require.Equal(t, uint64(1), totals.VoidedCount)
	request.ExpectedRound = math.MaxUint64
	require.NoError(t, k.RequestVoid(ctx, request))
	unchanged, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, ordered, unchanged)

	for _, vote := range quorum(g.Policies[0], types.OutcomeMinted) {
		require.NoError(t, k.ReportOutcome(ctx, &types.MsgReportOutcome{
			Reporter:      vote.Reporter,
			ID:            id,
			PolicyVersion: 1,
			Round:         1,
			Observation:   vote.Observation,
		}))
	}

	minted, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.StatusMinted, minted.Status)
	require.Equal(t, receipt, *minted.Settlement.VoidAuthorization)

	for _, member := range g.Policies[0].Members {
		e := testEvidence()
		require.NoError(t, k.ReportOutcome(ctx, &types.MsgReportOutcome{
			Reporter:      member,
			ID:            id,
			PolicyVersion: 1,
			Round:         1,
			Observation: types.Observation{
				Outcome:              types.OutcomeMinted,
				Slot:                 e.Slot,
				TransactionSignature: e.TransactionSignature,
			},
		}))
	}

	frozen, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, minted, frozen)
	p := g.Params
	p.Reporters = []string{testOwner(2).String()}
	require.NoError(t, k.SetParams(ctx, p))

	e := testEvidence()
	opposite := &types.MsgReportOutcome{
		Reporter:      p.Reporters[0],
		ID:            id,
		PolicyVersion: 2,
		Round:         1,
		Observation: types.Observation{
			Outcome:              types.OutcomeVoided,
			Slot:                 e.Slot,
			TransactionSignature: e.TransactionSignature,
		},
	}

	require.NoError(t, k.ReportOutcome(ctx, opposite))
	conflict, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.HoldConflict, conflict.Settlement.Hold)
	require.Equal(t, receipt, *conflict.Settlement.VoidAuthorization)

	required := g.Policies[0].Threshold.MulInt64(int64(len(g.Policies[0].Members))).Ceil().TruncateInt64()
	require.Len(t, conflict.Settlement.Votes, int(required)+1)

	opposite.Observation.Slot++
	require.NoError(t, k.ReportOutcome(ctx, opposite))
	frozen, err = k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, conflict, frozen)
	require.NoError(t, k.Resolve(ctx, &types.MsgConfirmVoid{
		Authority:     k.authority,
		ID:            id,
		ExpectedRound: 1,
		Evidence:      e,
		Reason:        "resolve conflict",
	}))

	require.NoError(t, k.Resolve(ctx, &types.MsgRestartTransfer{
		Authority:     k.authority,
		ID:            id,
		ExpectedRound: 2,
		Reason:        "restart ordered attempt",
	}))

	restarted, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.StatusVoided, restarted.Status)
	require.Equal(t, receipt, *restarted.Settlement.VoidAuthorization)
	require.NoError(t, k.Resolve(ctx, &types.MsgConfirmVoid{
		Authority:     k.authority,
		ID:            id,
		ExpectedRound: 3,
		Evidence:      e,
		Reason:        "confirm final void",
	}))

	authorized, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	p.Reporters = []string{testOwner(3).String()}
	require.NoError(t, k.SetParams(ctx, p))

	retained, err := k.GetTransfer(ctx, id)
	require.NoError(t, err)
	require.Equal(t, authorized, retained)
	exported, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	k2, ctx2, bank2 := testStore(t)
	ctx2 = ctx2.WithBlockHeight(ctx.BlockHeight() + 1).WithBlockTime(time.Unix(authorized.Settlement.ChallengeEnd, 0))
	importTestGenesis(t, k2, ctx2, bank2, exported)
	require.NoError(t, k2.FinalizeRefund(ctx2, &types.MsgFinalizeRefund{
		Executor: testOwner(1).String(),
		ID:       id,
	}))

	paid, err := k2.GetTransfer(ctx2, id)
	require.NoError(t, err)
	require.Equal(t, types.StatusRefunded, paid.Status)
	require.Equal(t, receipt, *paid.Settlement.VoidAuthorization)
	require.Equal(t, sdk.NewCoins(testCoin(10)), bank2.GetAllBalances(ctx2, testOwner(6)))
	require.NoError(t, k2.CheckBalance(ctx2))
}

func TestOwnerPaginationMergesAllFiveCanonicalBuckets(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()
	for _, reverse := range []bool{false, true} {
		var cursor []byte
		var got []types.ID
		for {
			page, err := q.Transfers(ctx, &types.QueryTransfersRequest{
				Owner: testOwner(6).String(),
				Pagination: &sdkquery.PageRequest{
					Limit:   1,
					Reverse: reverse,
					Key:     cursor,
				},
			})
			require.NoError(t, err)

			for _, record := range page.Transfers {
				got = append(got, record.ID)
			}

			if page.Pagination != nil {
				cursor = page.Pagination.NextKey
			} else {
				cursor = nil
			}

			if len(cursor) == 0 {
				break
			}
		}

		require.Len(t, got, 6)

		for i, id := range got {
			expected := uint64(2*i + 1)
			if reverse {
				expected = 12 - expected
			}

			require.Equal(t, expected, id.Sequence)
		}
	}

	key, _ := SettlementKey(*types.NewSettlementID(types.StatusVoided, g.Records[2].ID))
	for _, cursor := range [][]byte{key[2:], bytes.Repeat([]byte{0xff}, RecordIDKey.Size(g.Records[2].ID)), append(key[3:], 0)} {
		_, err := q.Transfers(ctx, &types.QueryTransfersRequest{
			Owner:      testOwner(6).String(),
			Pagination: &sdkquery.PageRequest{Key: cursor},
		})
		require.Error(t, err)
	}

	_, err := q.Transfers(ctx, &types.QueryTransfersRequest{
		Status:     types.StatusMinted,
		Pagination: &sdkquery.PageRequest{Key: key[2:]},
	})
	require.Error(t, err)
}
