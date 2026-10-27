package keeper

import (
	"math"
	"testing"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/sink/v1"
)

func openTestStore(t *testing.T) (Keeper, sdk.Context, *storetypes.KVStoreKey) {
	t.Helper()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	ctx := testutil.DefaultContext(key, storetypes.NewTransientStoreKey("transient_sink"))
	cdc := codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
	return NewKeeper(cdc, key, nil, nil, "authority"), ctx, key
}

func openTestKeeper(t *testing.T) (Keeper, sdk.Context) {
	t.Helper()
	k, ctx, _ := openTestStore(t)
	return k, ctx
}

// putTransfer writes a record and settlement directly, indexing it as open
// unless the status is terminal.
func putTransfer(t *testing.T, k Keeper, ctx sdk.Context, id uint64, status types.Status) {
	t.Helper()
	record, err := types.Transfer{ID: id, Recipient: [32]byte{1}, Amount: 1, Height: 1}.Marshal()
	require.NoError(t, err)
	require.NoError(t, k.transfers.Set(ctx, id, record))
	require.NoError(t, k.settlements.Set(ctx, id, types.Settlement{Status: status, Round: 1}))
	if status != types.Status_STATUS_MINTED && status != types.Status_STATUS_REFUNDED {
		require.NoError(t, k.open.Set(ctx, id))
	}
}

func openIDs(t *testing.T, transfers []types.TransferState) []uint64 {
	t.Helper()
	ids := make([]uint64, 0, len(transfers))
	for _, entry := range transfers {
		transfer, err := types.DecodeTransfer(entry.Record)
		require.NoError(t, err)
		ids = append(ids, transfer.ID)
	}
	return ids
}

func idRange(from, to uint64) []uint64 {
	var ids []uint64
	for id := from; id <= to; id++ {
		ids = append(ids, id)
	}
	return ids
}

func TestOpenTransfersPagesInIDOrder(t *testing.T) {
	k, ctx := openTestKeeper(t)
	q := QueryServer{Keeper: k}
	res, err := q.OpenTransfers(ctx, &types.QueryOpenTransfersRequest{})
	require.NoError(t, err)
	require.Empty(t, res.Transfers)
	require.Zero(t, res.NextStartAfter)

	// Decimal store keys would sort 10 before 2; the index must not.
	for id := uint64(1); id <= 620; id++ {
		status := types.Status_STATUS_PENDING
		switch id % 10 {
		case 3:
			status = types.Status_STATUS_VOIDED
		case 4:
			status = types.Status_STATUS_MINTED
		case 5:
			status = types.Status_STATUS_REFUNDED
		}
		putTransfer(t, k, ctx, id, status)
	}
	var open []uint64
	for id := uint64(1); id <= 620; id++ {
		if id%10 != 4 && id%10 != 5 {
			open = append(open, id)
		}
	}
	require.Len(t, open, 496)

	for _, tc := range []struct {
		name       string
		startAfter uint64
		limit      uint32
		want       []uint64
		next       uint64
	}{
		{"zero limit means 100", 0, 0, open[:100], open[99]},
		{"limit above 500 is capped", 0, 501, open[:496], 0},
		{"page in the middle", 1, 3, []uint64{2, 3, 6}, 6},
		{"start after a closed id", 4, 2, []uint64{6, 7}, 7},
		{"exact remainder has no next", open[489], 6, open[490:], 0},
		{"one less than the remainder", open[489], 5, open[490:495], open[494]},
		{"past the end", 620, 10, []uint64{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := q.OpenTransfers(ctx, &types.QueryOpenTransfersRequest{StartAfter: tc.startAfter, Limit: tc.limit})
			require.NoError(t, err)
			require.Equal(t, tc.want, openIDs(t, res.Transfers))
			require.Equal(t, tc.next, res.NextStartAfter)
		})
	}

	// Following next_start_after visits every open transfer exactly once.
	var seen []uint64
	var cursor uint64
	for {
		res, err := q.OpenTransfers(ctx, &types.QueryOpenTransfersRequest{StartAfter: cursor, Limit: 7})
		require.NoError(t, err)
		seen = append(seen, openIDs(t, res.Transfers)...)
		if res.NextStartAfter == 0 {
			break
		}
		cursor = res.NextStartAfter
	}
	require.Equal(t, open, seen)

	// More than 500 open transfers: the cap applies and paging continues.
	for id := uint64(621); id <= 700; id++ {
		putTransfer(t, k, ctx, id, types.Status_STATUS_PENDING)
	}
	all := append(append([]uint64(nil), open...), idRange(621, 700)...)
	res, err = q.OpenTransfers(ctx, &types.QueryOpenTransfersRequest{Limit: math.MaxUint32})
	require.NoError(t, err)
	require.Equal(t, all[:500], openIDs(t, res.Transfers))
	require.Equal(t, all[499], res.NextStartAfter)
	res, err = q.OpenTransfers(ctx, &types.QueryOpenTransfersRequest{StartAfter: res.NextStartAfter, Limit: 500})
	require.NoError(t, err)
	require.Equal(t, all[500:], openIDs(t, res.Transfers))
	require.Zero(t, res.NextStartAfter)

	// A nil request is the default page.
	res, err = q.OpenTransfers(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, open[:100], openIDs(t, res.Transfers))
}

func TestOpenTransfersAtMaximumID(t *testing.T) {
	k, ctx := openTestKeeper(t)
	putTransfer(t, k, ctx, math.MaxUint64-1, types.Status_STATUS_PENDING)
	putTransfer(t, k, ctx, math.MaxUint64, types.Status_STATUS_VOIDED)
	transfers, next, err := k.OpenTransfers(ctx, math.MaxUint64-2, 1)
	require.NoError(t, err)
	require.Equal(t, []uint64{math.MaxUint64 - 1}, openIDs(t, transfers))
	require.Equal(t, uint64(math.MaxUint64-1), next)
	transfers, next, err = k.OpenTransfers(ctx, next, 1)
	require.NoError(t, err)
	require.Equal(t, []uint64{math.MaxUint64}, openIDs(t, transfers))
	require.Zero(t, next)
	transfers, next, err = k.OpenTransfers(ctx, math.MaxUint64, 1)
	require.NoError(t, err)
	require.Empty(t, transfers)
	require.Zero(t, next)
}

func TestOpenTransfersRejectsInconsistentIndex(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(Keeper, sdk.Context, *storetypes.KVStoreKey)
	}{
		{"missing record", func(k Keeper, ctx sdk.Context, _ *storetypes.KVStoreKey) { require.NoError(t, k.open.Set(ctx, 9)) }},
		{"terminal settlement", func(k Keeper, ctx sdk.Context, _ *storetypes.KVStoreKey) {
			putTransfer(t, k, ctx, 9, types.Status_STATUS_MINTED)
			require.NoError(t, k.open.Set(ctx, 9))
		}},
		{"malformed key", func(_ Keeper, ctx sdk.Context, key *storetypes.KVStoreKey) {
			// An ID shorter than eight bytes cannot be decoded.
			ctx.KVStore(key).Set(append(append([]byte{}, OpenTransfersKey.Bytes()...), 0, 9), []byte{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, ctx, key := openTestStore(t)
			putTransfer(t, k, ctx, 1, types.Status_STATUS_PENDING)
			tc.setup(k, ctx, key)
			_, err := QueryServer{Keeper: k}.OpenTransfers(ctx, &types.QueryOpenTransfersRequest{})
			require.Equal(t, codes.Internal, status.Code(err))
		})
	}
}
