package app_test

import (
	"bytes"
	"context"
	"slices"
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

func storeEntries(s storetypes.KVStore, prefix []byte) map[string][]byte {
	entries := make(map[string][]byte)
	it := storetypes.KVStorePrefixIterator(s, prefix)
	defer func() { _ = it.Close() }()
	for ; it.Valid(); it.Next() {
		entries[string(it.Key())] = bytes.Clone(it.Value())
	}
	return entries
}

func transferID(t *testing.T, entry types.TransferState) uint64 {
	t.Helper()
	transfer, err := types.DecodeTransfer(entry.Record)
	require.NoError(t, err)
	return transfer.ID
}

// checkOpenIndex pages through OpenTransfers one entry at a time and requires
// exactly the non-terminal transfers, in ascending ID order, with their full
// current state. It returns the open IDs.
func (h *sinkHarness) checkOpenIndex(t *testing.T, ctx sdk.Context) []uint64 {
	t.Helper()
	k := h.a.Keepers.Akash.Sink
	want := []uint64{}
	for _, entry := range k.ExportGenesis(ctx).Transfers {
		if !entry.Settlement.Terminal() {
			want = append(want, transferID(t, entry))
		}
	}
	slices.Sort(want)
	require.Equal(t, uint64(len(want)), k.GetTotals(ctx).PendingCount)
	q := sinkkeeper.QueryServer{Keeper: k}
	got := []uint64{}
	var cursor uint64
	for {
		res, err := q.OpenTransfers(ctx, &types.QueryOpenTransfersRequest{StartAfter: cursor, Limit: 1})
		require.NoError(t, err)
		require.LessOrEqual(t, len(res.Transfers), 1)
		for _, entry := range res.Transfers {
			id := transferID(t, entry)
			current, ok := k.GetTransfer(ctx, id)
			require.True(t, ok)
			require.Equal(t, current, entry)
			got = append(got, id)
		}
		if res.NextStartAfter == 0 {
			break
		}
		cursor = res.NextStartAfter
	}
	require.Equal(t, want, got)
	return got
}

// The open index follows every settlement path: mint quorum, void quorum and
// refund, vetoes, conflicts, governance confirmation and restart.
func TestSinkOpenTransfersFollowSettlement(t *testing.T) {
	h := newSinkHarness(t, func(g *types.GenesisState) {
		g.Params.MaxTransfer = 100
		g.Params.DailyRefundCap = 100
		g.Params.ChallengePeriodSeconds = types.DefaultChallengePeriodSeconds
		g.Params.RefundsEnabled = true
		g.Params.MaxDailyPerAccount = 50
		g.Params.MaxPendingPerAccount = 50
	})
	k := h.a.Keepers.Akash.Sink
	require.Empty(t, h.checkOpenIndex(t, h.ctx()))
	for i := 0; i < 12; i++ {
		res := h.deliver(t, "", h.deposit(10))
		require.Zero(t, res.Code, res.Log)
	}
	// Ascending numeric order, not decimal key order (10 before 2).
	require.Equal(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, h.checkOpenIndex(t, h.ctx()))
	// A simulated deposit leaves no index entry.
	_, _, err := h.a.Simulate(h.tx(t, "", h.deposit(10)))
	require.NoError(t, err)
	require.Len(t, h.checkOpenIndex(t, h.ctx()), 12)

	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	h.quorum(t, 2, types.Outcome_OUTCOME_MINTED) // minted: closed
	h.quorum(t, 3, types.Outcome_OUTCOME_VOIDED) // voided, then refunded below
	require.Contains(t, h.checkOpenIndex(t, h.ctx()), uint64(3))
	h.quorum(t, 4, types.Outcome_OUTCOME_VOIDED) // voided and vetoed: held, open
	member := sdk.AccAddress(h.reporters[3].PubKey().Address()).String()
	res := h.deliverBytes(t, h.txFrom(t, h.reporters[3], "", &types.MsgVetoVoid{Signer: member, Id: 4, Reason: "disputed"}))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 5, types.Outcome_OUTCOME_MINTED) // minted, then a conflict hold: closed
	res = h.report(t, 3, 5, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.Hold_HOLD_CONFLICT, h.settlement(t, 5).Hold)
	require.NoError(t, h.resolve(t, 6, types.Decision_DECISION_CONFIRM_MINT)) // closed
	h.quorum(t, 7, types.Outcome_OUTCOME_VOIDED)                              // restarted: open
	require.NoError(t, h.resolve(t, 7, types.Decision_DECISION_RESTART))
	require.NoError(t, k.RequestVoid(h.ctx(), gov, 8)) // void order only: open
	require.NoError(t, k.RequestVoid(h.ctx(), gov, 9))
	require.NoError(t, h.resolve(t, 9, types.Decision_DECISION_CONFIRM_VOID)) // voided: open
	h.quorum(t, 11, types.Outcome_OUTCOME_MINTED)                             // closed
	require.Equal(t, []uint64{1, 3, 4, 7, 8, 9, 10, 12}, h.checkOpenIndex(t, h.ctx()))

	h.now = time.Unix(h.settlement(t, 3).ChallengeEnd, 0)
	res = h.refund(t, 3)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.Status_STATUS_REFUNDED, h.settlement(t, 3).Status)
	res = h.refund(t, 3) // A repeated refund changes nothing.
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []uint64{1, 4, 7, 8, 9, 10, 12}, h.checkOpenIndex(t, h.ctx()))

	// The registered gRPC route serves committed state.
	request, err := (&types.QueryOpenTransfersRequest{StartAfter: 4, Limit: 2}).Marshal()
	require.NoError(t, err)
	query, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/akash.sink.v1.Query/OpenTransfers", Data: request, Height: h.height})
	require.NoError(t, err)
	require.Zero(t, query.Code, query.Log)
	var page types.QueryOpenTransfersResponse
	require.NoError(t, page.Unmarshal(query.Value))
	require.Len(t, page.Transfers, 2)
	require.Equal(t, uint64(7), transferID(t, page.Transfers[0]))
	require.Equal(t, uint64(8), transferID(t, page.Transfers[1]))
	require.Equal(t, uint64(8), page.NextStartAfter)
	require.True(t, page.Transfers[1].VoidOrdered)

	h.checkSettlementExport(t)
}
