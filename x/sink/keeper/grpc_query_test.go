package keeper

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/util/query"
)

// remapOwners replaces the owners of populatedGenesis with addresses of the given length.
func remapOwners(g *types.GenesisState, length int) map[string]string {
	owners := map[string]string{
		testOwner(6).String(): sdk.AccAddress(bytes.Repeat([]byte{6}, length)).String(),
		testOwner(7).String(): sdk.AccAddress(bytes.Repeat([]byte{7}, length)).String(),
	}

	for i := range g.Records {
		if owner, ok := owners[g.Records[i].ID.Owner]; ok {
			g.Records[i].ID.Owner = owner
		}
	}

	for i := range g.Accounts {
		if owner, ok := owners[g.Accounts[i].Owner]; ok {
			g.Accounts[i].Owner = owner
		}
	}

	return owners
}

func transferIDs(transfers []types.TransferState) []types.ID {
	ids := make([]types.ID, 0, len(transfers))
	for _, t := range transfers {
		ids = append(ids, t.ID)
	}

	return ids
}

func TestQueryTransfersCursorOrderAndFilters(t *testing.T) {
	for _, length := range []int{20, 32} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			testQueryTransfersCursorOrderAndFilters(t, length)
		})
	}
}

func testQueryTransfersCursorOrderAndFilters(t *testing.T, length int) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	owners := remapOwners(g, length)
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()

	for _, filter := range []types.QueryTransfersRequest{
		{},
		{Owner: owners[testOwner(6).String()]},
		{Status: types.StatusTransferPending},
		{
			Status: types.StatusRefundPending,
			Owner:  owners[testOwner(7).String()],
		},
	} {
		history := filter.Owner == "" && filter.Status == types.StatusInvalid

		// Expected cursor contents: the statuses still to walk, the path marker
		// and, per record, the primary key a cursor resuming at it carries.
		states, marker := []byte{1, 2, 3, 4, 5}, []byte{1}
		switch {
		case history:
			states, marker = []byte{0}, []byte{2}
		case filter.Owner == "":
			states, marker = []byte{byte(filter.Status)}, nil
		case filter.Status != types.StatusInvalid:
			states = []byte{byte(filter.Status)}
		}

		type step struct {
			id types.ID
			pk []byte
		}

		var expected []step
		for _, record := range g.Records {
			if filter.Owner != "" && record.ID.Owner != filter.Owner || filter.Status != types.StatusInvalid && record.Status != filter.Status {
				continue
			}

			owner := sdk.MustAccAddressFromBech32(record.ID.Owner)
			identity := append([]byte{byte(len(owner))}, owner...)                   //nolint:gosec // Native address lengths fit one byte.
			position := binary.BigEndian.AppendUint64(nil, uint64(record.ID.Height)) //nolint:gosec // Fixture heights are positive.
			position = binary.BigEndian.AppendUint64(position, record.ID.Sequence)

			pk := append(bytes.Clone(identity), position...)
			if history {
				pk = append(bytes.Clone(position), identity...)
			}

			expected = append(expected, step{id: record.ID, pk: pk})
		}

		slices.SortFunc(expected, func(a, b step) int { return bytes.Compare(a.pk, b.pk) })
		require.GreaterOrEqual(t, len(expected), 2)

		filter.Owner = strings.ToUpper(filter.Owner)
		for _, reverse := range []bool{false, true} {
			if reverse {
				slices.Reverse(expected)
			}

			var cursor, first []byte
			for i, want := range expected {
				filter.Pagination = &sdkquery.PageRequest{
					Key:     cursor,
					Limit:   1,
					Reverse: reverse,
				}
				page, err := q.Transfers(ctx, &filter)
				require.NoError(t, err)
				require.Len(t, page.Transfers, 1)
				require.Equal(t, want.id, page.Transfers[0].ID)
				require.Equal(t, uint64(1), page.Pagination.Total)

				cursor = page.Pagination.NextKey
				if i+1 == len(expected) {
					require.Empty(t, cursor)
					continue
				}

				if first == nil {
					first = cursor
				}

				gotStates, prefix, pk, unsolicited, err := query.DecodePaginationKey(cursor)
				require.NoError(t, err)
				require.Equal(t, states, gotStates)
				require.Equal(t, states[:1], prefix)
				require.Equal(t, expected[i+1].pk, pk)
				require.Equal(t, marker, unsolicited)
			}

			for _, bad := range [][]byte{append(bytes.Clone(first), 0), append([]byte{first[0] ^ 0xff}, first[1:]...), expected[0].pk} {
				filter.Pagination = &sdkquery.PageRequest{Key: bad, Reverse: reverse}
				_, err := q.Transfers(ctx, &filter)
				require.Equal(t, codes.InvalidArgument, status.Code(err))
			}
		}
	}
}

// util/query prefixes each cursor part with one length byte, so a cursor can
// carry record and history keys of owners up to 238 bytes (1 + 238 + 16 = 255).
func TestQueryTransfersCursorKeyLimit(t *testing.T) {
	for _, tc := range []struct {
		length int
		code   codes.Code
	}{
		{238, codes.OK},
		{239, codes.Internal},
	} {
		t.Run(strconv.Itoa(tc.length), func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			owners := remapOwners(g, tc.length)
			importTestGenesis(t, k, ctx, bank, g)
			q := k.NewQuerier()

			for _, filter := range []types.QueryTransfersRequest{
				{},
				{Owner: owners[testOwner(6).String()]},
				{Status: types.StatusRefundPending},
				{
					Owner:  owners[testOwner(6).String()],
					Status: types.StatusRefundPending,
				},
			} {
				filter.Pagination = &sdkquery.PageRequest{Limit: 1}
				require.NotPanics(t, func() {
					page, err := q.Transfers(ctx, &filter)
					require.Equal(t, tc.code, status.Code(err))
					if err == nil {
						require.NotEmpty(t, page.Pagination.NextKey)
					}
				})
			}
		})
	}
}

func TestQueryTransfersRejectsForgedCursors(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()

	record, err := encodeKey(RecordIDKey, g.Records[0].ID)
	require.NoError(t, err)
	chrono, err := encodeKey(chronoIDKey, g.Records[0].ID)
	require.NoError(t, err)

	cursor := func(states, prefix, pk, unsolicited []byte) []byte {
		key, err := query.EncodePaginationKey(states, prefix, pk, unsolicited)
		require.NoError(t, err)

		return key
	}

	// EncodePaginationKey refuses empty states, so build that envelope by hand.
	data := append([]byte{0, 1, 1, byte(len(record))}, record...)
	noStates := append(binary.BigEndian.AppendUint32(nil, crc32.ChecksumIEEE(data)), data...)

	for name, key := range map[string][]byte{
		"no states":              noStates,
		"unknown marker":         cursor([]byte{1}, []byte{1}, record, []byte{3}),
		"zero marker":            cursor([]byte{1}, []byte{1}, record, []byte{0}),
		"two marker bytes":       cursor([]byte{1}, []byte{1}, record, []byte{1, 1}),
		"prefix mismatch":        cursor([]byte{1}, []byte{2}, record, nil),
		"bucket of two statuses": cursor([]byte{1, 2}, []byte{1}, record, nil),
		"unknown status":         cursor([]byte{6}, []byte{6}, record, nil),
		"owner status subset":    cursor([]byte{1, 3}, []byte{1}, record, []byte{1}),
		"history with a status":  cursor([]byte{1}, []byte{1}, chrono, []byte{2}),
		"history record key":     cursor([]byte{0}, []byte{0}, record, []byte{2}),
		"truncated record key":   cursor([]byte{1}, []byte{1}, record[:len(record)-1], nil),
		"trailing key bytes":     cursor([]byte{1}, []byte{1}, append(bytes.Clone(record), 0), nil),
	} {
		require.NotPanics(t, func() {
			_, err := q.Transfers(ctx, &types.QueryTransfersRequest{Pagination: &sdkquery.PageRequest{Key: key}})
			require.Equal(t, codes.InvalidArgument, status.Code(err), name)
		}, name)
	}
}

func TestQueryStoredStateFailuresAreInternal(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()

	usage, err := collections.EncodeKeyWithPrefix(AccountsKey.Bytes(), sdk.AccAddressKey, testOwner(6))
	require.NoError(t, err)
	settlement, err := SettlementKey(types.NewSettlementID(g.Records[0].Status, g.Records[0].ID))
	require.NoError(t, err)
	// A refund queue entry under the governance scope whose deadline is truncated.
	refund := append(append(bytes.Clone(RefundQueueKey.Bytes()), make([]byte, 8)...), 1)
	current, err := collections.EncodeKeyWithPrefix(PoliciesKey.Bytes(), collections.Uint64Key, g.PolicySequence)
	require.NoError(t, err)
	first, err := collections.EncodeKeyWithPrefix(PoliciesKey.Bytes(), collections.Uint64Key, uint64(1))
	require.NoError(t, err)
	// populatedGenesis holds its second record under a conflict.
	disputed, err := TransferKey(g.Records[1].ID)
	require.NoError(t, err)

	state := func(ctx sdk.Context) error {
		_, err := q.State(ctx, nil)
		return err
	}

	reporters := func(version uint64) func(sdk.Context) error {
		return func(ctx sdk.Context) error {
			_, err := q.Reporters(ctx, &types.QueryReportersRequest{Version: version})
			return err
		}
	}

	for _, tc := range []struct {
		name  string
		key   []byte
		query func(sdk.Context) error
	}{
		{"totals", TotalsKey.Bytes(), state},
		{"send usage", SendUsageKey.Bytes(), state},
		{"refund usage", RefundUsageKey.Bytes(), state},
		{"policy sequence", PolicySequenceKey.Bytes(), state},
		{"refund queue", refund, state},
		{"account usage", usage, func(ctx sdk.Context) error {
			_, err := q.Account(ctx, &types.QueryAccountRequest{Owner: testOwner(6).String()})
			return err
		}},
		{"settlement", settlement, func(ctx sdk.Context) error {
			_, err := q.Transfer(ctx, &types.QueryTransferRequest{ID: g.Records[0].ID})
			return err
		}},
		{"current policy", current, reporters(0)},
		{"policy version", first, reporters(1)},
		{"policy history", first, reporters(math.MaxUint64)},
		{"disputed record", disputed, func(ctx sdk.Context) error {
			_, err := q.Disputes(ctx, nil)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache, _ := ctx.CacheContext()
			cache.KVStore(k.skey).Set(tc.key, []byte{0xff, 0xff})
			require.Equal(t, codes.Internal, status.Code(tc.query(cache)))
		})
	}
}

func TestQueryTransfersResumeFollowsCursor(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()

	first, err := q.Transfers(ctx, &types.QueryTransfersRequest{
		Owner:      testOwner(6).String(),
		Status:     types.StatusRefundPending,
		Pagination: &sdkquery.PageRequest{Limit: 1},
	})
	require.NoError(t, err)
	require.Equal(t, []types.ID{g.Records[6].ID}, transferIDs(first.Transfers))

	// The cursor carries the path, owner and statuses; request filters are ignored on resume.
	next, err := q.Transfers(ctx, &types.QueryTransfersRequest{
		Owner:      testOwner(7).String(),
		Pagination: &sdkquery.PageRequest{Key: first.Pagination.NextKey},
	})
	require.NoError(t, err)
	require.Equal(t, []types.ID{g.Records[8].ID}, transferIDs(next.Transfers))
	require.Empty(t, next.Pagination.NextKey)
}

func TestQueryTransfersPaginationRules(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := historyGenesis(1, settlementIn(types.StatusTransferPending), 10, 105)
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()

	for _, filter := range []types.QueryTransfersRequest{
		{},
		{Owner: g.Records[0].ID.Owner},
		{Status: types.StatusMinted},
	} {
		page, err := q.Transfers(ctx, &filter)
		require.NoError(t, err)
		require.Len(t, page.Transfers, sdkquery.DefaultLimit)
		require.Equal(t, uint64(sdkquery.DefaultLimit), page.Pagination.Total)
		require.NotEmpty(t, page.Pagination.NextKey)

		all := len(g.Records)
		if filter.Status == types.StatusMinted {
			all--
		}

		// There is no maximum page size.
		for _, limit := range []uint64{500, 501} {
			filter.Pagination = &sdkquery.PageRequest{Limit: limit}
			res, err := q.Transfers(ctx, &filter)
			require.NoError(t, err)
			require.Len(t, res.Transfers, all)
			require.Empty(t, res.Pagination.NextKey)
		}

		// count_total is ignored; Total counts the returned page.
		filter.Pagination = &sdkquery.PageRequest{CountTotal: true}
		res, err := q.Transfers(ctx, &filter)
		require.NoError(t, err)
		require.Len(t, res.Transfers, sdkquery.DefaultLimit)
		require.Equal(t, uint64(sdkquery.DefaultLimit), res.Pagination.Total)

		// Offset requires a status filter.
		filter.Pagination = &sdkquery.PageRequest{Offset: 1}
		res, err = q.Transfers(ctx, &filter)
		if filter.Status == types.StatusInvalid {
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			continue
		}

		require.NoError(t, err)
		require.Equal(t, transferIDs(page.Transfers[1:]), transferIDs(res.Transfers[:len(page.Transfers)-1]))
	}
}

func TestQueryDisputesNativePagination(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()

	// populatedGenesis records share one height with ascending sequences, so
	// record order is chronological order.
	var expected []types.ID
	for _, record := range g.Records {
		if record.Settlement.Hold != types.HoldNone {
			expected = append(expected, record.ID)
		}
	}
	require.Len(t, expected, 6)

	all, err := q.Disputes(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, expected, transferIDs(all.Transfers))

	var got []types.ID
	var cursor []byte
	for {
		page, err := q.Disputes(ctx, &types.QueryDisputesRequest{Pagination: &sdkquery.PageRequest{
			Key:   cursor,
			Limit: 4,
		}})
		require.NoError(t, err)
		got = append(got, transferIDs(page.Transfers)...)

		cursor = page.Pagination.NextKey
		if len(cursor) == 0 {
			break
		}

		// The cursor is the native disputes index key of the next dispute.
		n, id, err := chronoIDKey.Decode(cursor)
		require.NoError(t, err)
		require.Equal(t, len(cursor), n)
		require.Equal(t, expected[len(got)], id)
	}
	require.Equal(t, expected, got)

	reverse, err := q.Disputes(ctx, &types.QueryDisputesRequest{Pagination: &sdkquery.PageRequest{
		Reverse: true,
		Limit:   1,
	}})
	require.NoError(t, err)
	require.Equal(t, expected[len(expected)-1:], transferIDs(reverse.Transfers))

	offset, err := q.Disputes(ctx, &types.QueryDisputesRequest{Pagination: &sdkquery.PageRequest{
		Offset:     2,
		Limit:      2,
		CountTotal: true,
	}})
	require.NoError(t, err)
	require.Equal(t, expected[2:4], transferIDs(offset.Transfers))
	require.Equal(t, uint64(len(expected)), offset.Pagination.Total)

	_, err = q.Disputes(ctx, &types.QueryDisputesRequest{Pagination: &sdkquery.PageRequest{
		Offset: 1,
		Key:    []byte{1},
	}})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestQueryReportersVersions(t *testing.T) {
	fresh, freshCtx, freshBank := testStore(t)
	importTestGenesis(t, fresh, freshCtx, freshBank, types.DefaultGenesis())
	for _, version := range []uint64{0, math.MaxUint64} {
		res, err := fresh.NewQuerier().Reporters(freshCtx, &types.QueryReportersRequest{Version: version})
		require.NoError(t, err)
		require.Empty(t, res.Reporters)
	}

	_, err := fresh.NewQuerier().Reporters(freshCtx, &types.QueryReportersRequest{Version: 1})
	require.Equal(t, codes.NotFound, status.Code(err))

	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()
	for version, want := range map[uint64][]types.ReporterPolicy{
		0:              {g.Policies[1]},
		1:              {g.Policies[0]},
		2:              {g.Policies[1]},
		math.MaxUint64: g.Policies,
	} {
		res, err := q.Reporters(ctx, &types.QueryReportersRequest{Version: version})
		require.NoError(t, err)
		require.Equal(t, want, res.Reporters, "version %d", version)
	}

	_, err = q.Reporters(ctx, &types.QueryReportersRequest{Version: 3})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestQueryRequestValidation(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	q := k.NewQuerier()

	// Requests without required fields accept nil as their defaults.
	params, err := q.Params(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, g.Params, params.Params)

	// Genesis usage is stamped at epoch 19; the store's native epoch is 20.
	state, err := q.State(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, g.Totals, state.Totals)
	require.Equal(t, uint64(20), state.Epoch)
	require.Equal(t, types.EpochUsage{Epoch: 20, Amount: types.ZeroAmount()}, state.SendUsage)
	require.Equal(t, types.EpochUsage{Epoch: 20, Amount: types.ZeroAmount()}, state.RefundUsage)
	require.Equal(t, &g.Records[6].ID, state.RefundHead)

	transfers, err := q.Transfers(ctx, nil)
	require.NoError(t, err)
	require.Len(t, transfers.Transfers, len(g.Records))

	disputes, err := q.Disputes(ctx, nil)
	require.NoError(t, err)
	require.Len(t, disputes.Transfers, 6)

	reporters, err := q.Reporters(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, []types.ReporterPolicy{g.Policies[1]}, reporters.Reporters)

	// Requests that need an ID or owner reject nil and malformed input.
	_, err = q.Transfer(ctx, nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = q.Transfer(ctx, &types.QueryTransferRequest{})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = q.Account(ctx, nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = q.Account(ctx, &types.QueryAccountRequest{Owner: "akash1invalid"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = q.Transfers(ctx, &types.QueryTransfersRequest{Owner: "akash1invalid"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = q.Transfers(ctx, &types.QueryTransfersRequest{Status: types.Status(9)})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	// Domain errors keep their module error.
	_, err = q.Transfer(ctx, &types.QueryTransferRequest{ID: types.ID{
		Owner:    testOwner(9).String(),
		Height:   1,
		Sequence: 1,
	}})
	require.ErrorIs(t, err, types.ErrTransferNotFound)

	got, err := q.Transfer(ctx, &types.QueryTransferRequest{ID: g.Records[2].ID})
	require.NoError(t, err)
	require.Equal(t, types.StatusVoided, got.Transfer.Status)

	account, err := q.Account(ctx, &types.QueryAccountRequest{Owner: testOwner(6).String()})
	require.NoError(t, err)
	require.Equal(t, uint64(20), account.Epoch)
	require.Equal(t, int64(20), account.Usage.LastEpoch)
	require.True(t, account.Usage.EpochSent.IsZero())
	require.Equal(t, testCoin(60), account.Usage.TotalSent)

	// An owner without usage reads as empty usage at the current epoch.
	unused, err := q.Account(ctx, &types.QueryAccountRequest{Owner: testOwner(5).String()})
	require.NoError(t, err)
	require.Equal(t, uint64(20), unused.Epoch)
	require.Equal(t, types.AccountUsage{
		TotalSent: types.ZeroAmount(),
		EpochSent: types.ZeroAmount(),
		LastEpoch: 20,
	}, unused.Usage)
}
