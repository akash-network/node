package sink_test

import (
	"context"
	"testing"
	"time"

	pruningtypes "cosmossdk.io/store/pruning/types"
	abci "github.com/cometbft/cometbft/abci/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestSinkOpenTransfersFollowSettlement(t *testing.T) {
	h := setupReportingTestSuite(t)
	for i := 0; i < 3; i++ {
		res := h.deliver(t, "", h.deposit(10))
		require.Zero(t, res.Code, res.Log)
	}

	h.quorum(t, 1, types.OutcomeMinted)
	h.quorum(t, 2, types.OutcomeVoided)
	q := h.a.Keepers.Akash.Sink.NewQuerier()
	for _, status := range []types.Status{types.StatusTransferPending, types.StatusMinted, types.StatusRefundPending} {
		res, err := q.Transfers(h.ctx(), &types.QueryTransfersRequest{
			Owner:  h.sender.String(),
			Status: status,
		})
		require.NoError(t, err)
		require.Len(t, res.Transfers, 1)
		require.Equal(t, status, res.Transfers[0].Status)
	}

	h.now = time.Unix(h.transfer(t, 2).Settlement.ChallengeEnd, 0).UTC()
	res := h.refund(t, 2)
	require.Zero(t, res.Code, res.Log)
	paid, err := q.Transfers(h.ctx(), &types.QueryTransfersRequest{Status: types.StatusRefunded})
	require.NoError(t, err)
	require.Len(t, paid.Transfers, 1)
	h.checkSettlementExport(t)
}

func TestSinkHistoricalPagesAndChronologicalDiscovery(t *testing.T) {
	h := setupTestSuite(t, nil)
	for i := 0; i < 3; i++ {
		res := h.deliver(t, "", h.deposit(10))
		require.Zero(t, res.Code, res.Log)
	}

	height := h.height
	query := func(req *types.QueryTransfersRequest, height int64) *types.QueryTransfersResponse {
		t.Helper()
		data, err := req.Marshal()
		require.NoError(t, err)
		res, err := h.a.Query(context.Background(), &abci.RequestQuery{
			Path:   "/akash.sink.v1.Query/Transfers",
			Data:   data,
			Height: height,
		})
		require.NoError(t, err)
		require.Zero(t, res.Code, res.Log)
		var out types.QueryTransfersResponse
		require.NoError(t, out.Unmarshal(res.Value))

		return &out
	}

	page := query(&types.QueryTransfersRequest{
		Pagination: &sdkquery.PageRequest{Limit: 2},
	}, height)
	require.Len(t, page.Transfers, 2)
	require.NotEmpty(t, page.Pagination.NextKey)
	res := h.depositFrom(t, h.reporters[4], 10)
	require.Zero(t, res.Code, res.Log)
	next := query(&types.QueryTransfersRequest{
		Pagination: &sdkquery.PageRequest{
			Key:   page.Pagination.NextKey,
			Limit: 2,
		},
	}, height)
	require.Len(t, next.Transfers, 1)
	require.Empty(t, next.Pagination.NextKey)
	current := query(&types.QueryTransfersRequest{
		Pagination: &sdkquery.PageRequest{
			Key:   page.Pagination.NextKey,
			Limit: 2,
		},
	}, h.height)
	require.Len(t, current.Transfers, 2)
	require.Equal(t, h.id(4), current.Transfers[1].ID)
	latest := query(&types.QueryTransfersRequest{}, h.height)
	require.Len(t, latest.Transfers, 4)
	data, err := (&types.QueryTransfersRequest{}).Marshal()
	require.NoError(t, err)
	bad, err := h.a.Query(context.Background(), &abci.RequestQuery{
		Path:   "/akash.sink.v1.Query/Transfers",
		Data:   data,
		Height: h.height + 100,
	})
	require.NoError(t, err)
	require.NotZero(t, bad.Code)
}

func TestSinkPrunedSnapshotFails(t *testing.T) {
	h := setupTestSuite(t, nil)
	h.a.CommitMultiStore().SetPruning(pruningtypes.NewPruningOptions(pruningtypes.PruningEverything))
	h.a.CommitMultiStore().SetIAVLSyncPruning(true)

	for i := 0; i < 12; i++ {
		res := h.deliver(t, "", h.deposit(1))
		require.Zero(t, res.Code, res.Log)
	}

	data, err := (&types.QueryTransfersRequest{
		Pagination: &sdkquery.PageRequest{Limit: 1},
	}).Marshal()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		response, err := h.a.Query(context.Background(), &abci.RequestQuery{
			Path:   "/akash.sink.v1.Query/Transfers",
			Data:   data,
			Height: 2,
		})
		return err == nil && response.Code != 0
	}, 5*time.Second, 10*time.Millisecond)
}
