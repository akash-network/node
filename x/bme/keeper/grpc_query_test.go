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
