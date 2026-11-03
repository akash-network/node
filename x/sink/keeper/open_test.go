package keeper

import (
	"testing"

	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestTransfersIndexedPagination(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := types.DefaultGenesis()
	for i := 1; i <= 620; i++ {
		owner := testOwner(byte(i%2 + 1)).String()
		g.Records = append(g.Records, types.GenesisRecord{Status: types.StatusTransferPending, ID: types.ID{Owner: owner, Height: int64(i), Sequence: 1}, Data: types.RecordData{Dest: types.SolanaAddress{1}.String(), Amount: testCoin(1)}, Settlement: types.SettlementData{Policy: g.Policies[0], Round: 1}})
	}

	g.Totals.PendingAmount = testCoin(620)
	g.Totals.PendingCount = 620
	g.SendUsage = types.EpochUsage{Epoch: 19, Amount: testCoin(620)}
	for n := byte(1); n <= 2; n++ {
		g.Accounts = append(g.Accounts, types.GenesisAccount{Owner: testOwner(n).String(), Usage: types.AccountUsage{TotalSent: testCoin(310), EpochSent: testCoin(310), LastEpoch: 19}})
	}

	ctx = ctx.WithBlockHeight(700)
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()
	for _, filter := range []types.QueryTransfersRequest{{}, {Owner: testOwner(1).String()}, {Status: types.StatusTransferPending}, {Owner: testOwner(2).String(), Status: types.StatusTransferPending}} {
		var cursor []byte
		seen := map[types.ID]bool{}
		lastHeight := int64(0)
		for {
			filter.Pagination = &sdkquery.PageRequest{Limit: 17, Key: cursor}
			res, err := q.Transfers(ctx, &filter)
			require.NoError(t, err)

			for _, entry := range res.Transfers {
				require.False(t, seen[entry.ID])
				seen[entry.ID] = true

				if filter.Status == types.StatusInvalid || filter.Owner != "" {
					require.Greater(t, entry.ID.Height, lastHeight)
				}

				lastHeight = entry.ID.Height
			}

			cursor = res.Pagination.NextKey
			if len(cursor) == 0 {
				break
			}
		}

		expected := 620
		if filter.Owner != "" {
			expected = 310
		}

		require.Len(t, seen, expected)
	}

	page, err := q.Transfers(ctx, &types.QueryTransfersRequest{})
	require.NoError(t, err)
	require.Len(t, page.Transfers, 100)
	page, err = q.Transfers(ctx, &types.QueryTransfersRequest{Pagination: &sdkquery.PageRequest{Limit: 500}})
	require.NoError(t, err)
	require.Len(t, page.Transfers, 500)
	reverse, err := q.Transfers(ctx, &types.QueryTransfersRequest{Pagination: &sdkquery.PageRequest{Reverse: true, Limit: 2}})
	require.NoError(t, err)
	require.Equal(t, int64(620), reverse.Transfers[0].ID.Height)

	for _, req := range []*sdkquery.PageRequest{{Key: []byte{1}}, {Limit: 501}, {Offset: 1}, {CountTotal: true}} {
		_, err = q.Transfers(ctx, &types.QueryTransfersRequest{Pagination: req})
		require.Error(t, err)
	}

	ownerPage, err := q.Transfers(ctx, &types.QueryTransfersRequest{Owner: testOwner(1).String(), Pagination: &sdkquery.PageRequest{Limit: 1}})
	require.NoError(t, err)
	_, err = q.Transfers(ctx, &types.QueryTransfersRequest{Owner: testOwner(2).String(), Pagination: &sdkquery.PageRequest{Key: ownerPage.Pagination.NextKey}})
	require.Error(t, err)
}
