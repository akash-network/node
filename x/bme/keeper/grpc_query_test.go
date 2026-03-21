package keeper_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/sdkutil"
	"pkg.akt.dev/go/testutil"

	types "pkg.akt.dev/go/node/bme/v1"

	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/bme/keeper"
)

type grpcTestSuite struct {
	*state.TestSuite
	t      *testing.T
	ctx    sdk.Context
	keeper keeper.Keeper

	queryClient types.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	ssuite := state.SetupTestSuite(t)

	suite := &grpcTestSuite{
		TestSuite: ssuite,
		t:         t,
		ctx:       ssuite.Context(),
		keeper:    ssuite.BmeKeeper(),
	}

	querier := suite.keeper.NewQuerier()

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.App().InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = types.NewQueryClient(queryHelper)

	return suite
}

type ledgerTestRecord struct {
	id      types.LedgerRecordID
	pending *types.LedgerPendingRecord
	record  *types.LedgerRecord
}

func seedLedgerRecords(t *testing.T, ctx sdk.Context, k keeper.Keeper) []ledgerTestRecord {
	t.Helper()

	srcA := testutil.AccAddress(t).String()
	srcB := testutil.AccAddress(t).String()
	dstA := testutil.AccAddress(t).String()
	dstB := testutil.AccAddress(t).String()

	records := []ledgerTestRecord{
		{
			id: types.LedgerRecordID{
				Denom:    sdkutil.DenomUakt,
				ToDenom:  sdkutil.DenomUact,
				Source:   srcA,
				Height:   1,
				Sequence: 1,
			},
			pending: &types.LedgerPendingRecord{
				Owner:       srcA,
				To:          dstA,
				CoinsToBurn: sdk.NewInt64Coin(sdkutil.DenomUakt, 1000),
				DenomToMint: sdkutil.DenomUact,
			},
		},
		{
			id: types.LedgerRecordID{
				Denom:    sdkutil.DenomUact,
				ToDenom:  sdkutil.DenomUakt,
				Source:   srcB,
				Height:   2,
				Sequence: 1,
			},
			pending: &types.LedgerPendingRecord{
				Owner:       srcB,
				To:          dstB,
				CoinsToBurn: sdk.NewInt64Coin(sdkutil.DenomUact, 500),
				DenomToMint: sdkutil.DenomUakt,
			},
		},
		{
			id: types.LedgerRecordID{
				Denom:    sdkutil.DenomUakt,
				ToDenom:  sdkutil.DenomUact,
				Source:   srcA,
				Height:   3,
				Sequence: 1,
			},
			record: &types.LedgerRecord{
				BurnedFrom: srcA,
				MintedTo:   dstA,
				Burner:     types.ModuleName,
				Minter:     types.ModuleName,
			},
		},
		{
			id: types.LedgerRecordID{
				Denom:    sdkutil.DenomUact,
				ToDenom:  sdkutil.DenomUakt,
				Source:   srcB,
				Height:   4,
				Sequence: 1,
			},
			record: &types.LedgerRecord{
				BurnedFrom: srcB,
				MintedTo:   dstB,
				Burner:     types.ModuleName,
				Minter:     types.ModuleName,
			},
		},
	}

	for _, r := range records {
		if r.pending != nil {
			err := k.AddLedgerPendingRecord(ctx, r.id, *r.pending)
			require.NoError(t, err)
		}
		if r.record != nil {
			err := k.AddLedgerRecord(ctx, r.id, *r.record)
			require.NoError(t, err)
		}
	}

	return records
}

func TestGRPCQueryLedgerRecords(t *testing.T) {
	suite := setupTest(t)
	records := seedLedgerRecords(t, suite.ctx, suite.keeper)

	// find source addresses from seeded data
	srcA := records[0].id.Source
	srcB := records[1].id.Source

	var req *types.QueryLedgerRecordsRequest

	testCases := []struct {
		msg     string
		req     func()
		expLen  int
		expPass bool
	}{
		{
			"query without any filters",
			func() {
				req = &types.QueryLedgerRecordsRequest{}
			},
			4,
			true,
		},
		{
			"query with status filter pending",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						Status: "ledger_record_status_pending",
					},
				}
			},
			2,
			true,
		},
		{
			"query with status filter executed",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						Status: "ledger_record_status_executed",
					},
				}
			},
			2,
			true,
		},
		{
			"query with source filter srcA",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						Source: srcA,
					},
				}
			},
			2,
			true,
		},
		{
			"query with source filter srcB",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						Source: srcB,
					},
				}
			},
			2,
			true,
		},
		{
			"query with denom filter uakt",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						Denom: sdkutil.DenomUakt,
					},
				}
			},
			2,
			true,
		},
		{
			"query with to_denom filter uakt",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						ToDenom: sdkutil.DenomUakt,
					},
				}
			},
			2,
			true,
		},
		{
			"query with combined status and source filter",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						Status: "ledger_record_status_pending",
						Source: srcA,
					},
				}
			},
			1,
			true,
		},
		{
			"query with non-matching source",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						Source: testutil.AccAddress(t).String(),
					},
				}
			},
			0,
			true,
		},
		{
			"query with pagination limit",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Pagination: &sdkquery.PageRequest{Limit: 2},
				}
			},
			2,
			true,
		},
		{
			"query with invalid status filter",
			func() {
				req = &types.QueryLedgerRecordsRequest{
					Filters: types.LedgerRecordFilters{
						Status: "invalid_status",
					},
				}
			},
			0,
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.req()
			ctx := suite.ctx

			res, err := suite.queryClient.LedgerRecords(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, tc.expLen, len(res.Records))
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestGRPCQueryLedgerRecordsReverse(t *testing.T) {
	suite := setupTest(t)
	_ = seedLedgerRecords(t, suite.ctx, suite.keeper)

	ctx := suite.ctx

	t.Run("reverse returns records in opposite order of forward", func(t *testing.T) {
		// Get forward order
		fwd, err := suite.queryClient.LedgerRecords(ctx, &types.QueryLedgerRecordsRequest{})
		require.NoError(t, err)

		// Get reverse order
		rev, err := suite.queryClient.LedgerRecords(ctx, &types.QueryLedgerRecordsRequest{
			Pagination: &sdkquery.PageRequest{Reverse: true},
		})
		require.NoError(t, err)
		require.Len(t, rev.Records, len(fwd.Records))

		// Reverse should be the exact opposite of forward
		for i := range fwd.Records {
			j := len(fwd.Records) - 1 - i
			require.Equal(t, fwd.Records[i].ID, rev.Records[j].ID,
				"reverse[%d] should equal forward[%d]", j, i)
		}
	})

	t.Run("reverse with pagination key (multi-page)", func(t *testing.T) {
		// Get all records in reverse for reference
		allRev, err := suite.queryClient.LedgerRecords(ctx, &types.QueryLedgerRecordsRequest{
			Pagination: &sdkquery.PageRequest{Reverse: true},
		})
		require.NoError(t, err)

		// First page: 2 records
		res1, err := suite.queryClient.LedgerRecords(ctx, &types.QueryLedgerRecordsRequest{
			Pagination: &sdkquery.PageRequest{Limit: 2, Reverse: true},
		})
		require.NoError(t, err)
		require.Len(t, res1.Records, 2)
		require.NotEmpty(t, res1.Pagination.NextKey)

		// Second page — Reverse is NOT set; it's encoded in the NextKey
		res2, err := suite.queryClient.LedgerRecords(ctx, &types.QueryLedgerRecordsRequest{
			Pagination: &sdkquery.PageRequest{Key: res1.Pagination.NextKey, Limit: 10},
		})
		require.NoError(t, err)
		require.Len(t, res2.Records, 2)

		// Paginated results should match full reverse query
		paginated := append(res1.Records, res2.Records...)
		require.Len(t, paginated, len(allRev.Records))
		for i := range paginated {
			require.Equal(t, allRev.Records[i].ID, paginated[i].ID,
				"paginated[%d] should match full reverse[%d]", i, i)
		}
	})

	t.Run("reverse with status filter", func(t *testing.T) {
		// Get forward with status filter
		fwd, err := suite.queryClient.LedgerRecords(ctx, &types.QueryLedgerRecordsRequest{
			Filters: types.LedgerRecordFilters{
				Status: "ledger_record_status_pending",
			},
			Pagination: &sdkquery.PageRequest{},
		})
		require.NoError(t, err)
		require.Len(t, fwd.Records, 2)

		// Get reverse with same status filter
		rev, err := suite.queryClient.LedgerRecords(ctx, &types.QueryLedgerRecordsRequest{
			Filters: types.LedgerRecordFilters{
				Status: "ledger_record_status_pending",
			},
			Pagination: &sdkquery.PageRequest{Reverse: true},
		})
		require.NoError(t, err)
		require.Len(t, rev.Records, 2)

		// Should be opposite order
		require.Equal(t, fwd.Records[0].ID, rev.Records[1].ID)
		require.Equal(t, fwd.Records[1].ID, rev.Records[0].ID)
	})
}

type ledgerFilterModifier struct {
	fieldName string
	f         func(id types.LedgerRecordID, filter types.LedgerRecordFilters) types.LedgerRecordFilters
	getField  func(id types.LedgerRecordID) interface{}
}

func TestGRPCQueryLedgerRecordsWithFilter(t *testing.T) {
	suite := setupTest(t)
	records := seedLedgerRecords(t, suite.ctx, suite.keeper)

	var ids []types.LedgerRecordID
	for _, r := range records {
		ids = append(ids, r.id)
	}

	modifiers := []ledgerFilterModifier{
		{
			"source",
			func(id types.LedgerRecordID, filter types.LedgerRecordFilters) types.LedgerRecordFilters {
				filter.Source = id.Source
				return filter
			},
			func(id types.LedgerRecordID) interface{} {
				return id.Source
			},
		},
		{
			"denom",
			func(id types.LedgerRecordID, filter types.LedgerRecordFilters) types.LedgerRecordFilters {
				filter.Denom = id.Denom
				return filter
			},
			func(id types.LedgerRecordID) interface{} {
				return id.Denom
			},
		},
		{
			"to_denom",
			func(id types.LedgerRecordID, filter types.LedgerRecordFilters) types.LedgerRecordFilters {
				filter.ToDenom = id.ToDenom
				return filter
			},
			func(id types.LedgerRecordID) interface{} {
				return id.ToDenom
			},
		},
	}

	ctx := suite.ctx

	// Test each modifier individually against each record
	for _, id := range ids {
		for _, m := range modifiers {
			req := &types.QueryLedgerRecordsRequest{
				Filters: m.f(id, types.LedgerRecordFilters{}),
			}

			res, err := suite.queryClient.LedgerRecords(ctx, req)

			require.NoError(t, err, "testing %v", m.fieldName)
			require.NotNil(t, res, "testing %v", m.fieldName)
			require.GreaterOrEqual(t, len(res.Records), 1, "testing %v", m.fieldName)

			for _, rec := range res.Records {
				require.Equal(t, m.getField(id), m.getField(rec.ID), "testing %v", m.fieldName)
			}
		}
	}

	// Test all 2^N combinations of modifiers
	limit := int(math.Pow(2, float64(len(modifiers))))

	bogusID := types.LedgerRecordID{
		Denom:    "bogus_denom",
		ToDenom:  "bogus_to_denom",
		Source:   testutil.AccAddress(t).String(),
		Height:   9999,
		Sequence: 9999,
	}

	for i := 0; i != limit; i++ {
		modifiersToUse := make([]bool, len(modifiers))
		for j := 0; j != len(modifiers); j++ {
			mask := int(math.Pow(2, float64(j)))
			modifiersToUse[j] = (mask & i) != 0
		}

		// Test with matching IDs
		for _, id := range ids {
			filter := types.LedgerRecordFilters{}
			msg := strings.Builder{}
			msg.WriteString("testing filtering on: ")
			for k, useModifier := range modifiersToUse {
				if !useModifier {
					continue
				}
				modifier := modifiers[k]
				filter = modifier.f(id, filter)
				msg.WriteString(modifier.fieldName)
				msg.WriteString(", ")
			}

			req := &types.QueryLedgerRecordsRequest{
				Filters: filter,
			}

			res, err := suite.queryClient.LedgerRecords(ctx, req)

			require.NoError(t, err, msg.String())
			require.NotNil(t, res, msg.String())
			require.GreaterOrEqual(t, len(res.Records), 1, msg.String())

			for _, rec := range res.Records {
				for k, useModifier := range modifiersToUse {
					if !useModifier {
						continue
					}
					m := modifiers[k]
					require.Equal(t, m.getField(id), m.getField(rec.ID), "testing %v", m.fieldName)
				}
			}
		}

		// Test with non-matching (bogus) ID
		filter := types.LedgerRecordFilters{}
		msg := strings.Builder{}
		msg.WriteString("testing filtering on (using non matching ID): ")
		for k, useModifier := range modifiersToUse {
			if !useModifier {
				continue
			}
			modifier := modifiers[k]
			filter = modifier.f(bogusID, filter)
			msg.WriteString(modifier.fieldName)
			msg.WriteString(", ")
		}

		req := &types.QueryLedgerRecordsRequest{
			Filters: filter,
		}

		res, err := suite.queryClient.LedgerRecords(ctx, req)

		require.NoError(t, err, msg.String())
		require.NotNil(t, res, msg.String())
		expected := 0
		if i == 0 {
			expected = len(ids)
		}
		require.Len(t, res.Records, expected, msg.String())
	}
}
