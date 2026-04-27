package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
	"pkg.akt.dev/go/testutil"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	oracletypes "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/testutil/state"
	oraclekeeper "pkg.akt.dev/node/v2/x/oracle/keeper"
)

type grpcTestSuite struct {
	t      *testing.T
	suite  *state.TestSuite
	ctx    sdk.Context
	keeper oraclekeeper.Keeper

	queryClient oracletypes.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	ssuite := state.SetupTestSuite(t)
	app := ssuite.App()

	suite := &grpcTestSuite{
		t:      t,
		suite:  ssuite,
		ctx:    ssuite.Context(),
		keeper: app.Keepers.Akash.Oracle,
	}

	querier := suite.keeper.NewQuerier()
	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, app.InterfaceRegistry())
	oracletypes.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = oracletypes.NewQueryClient(queryHelper)

	return suite
}

func addPriceEntry(t *testing.T, ctx sdk.Context, keeper oraclekeeper.Keeper, source sdk.AccAddress, dataID oracletypes.DataID, height int64, timestamp time.Time, price sdkmath.LegacyDec) sdk.Context {
	ctx = ctx.WithBlockHeight(height).WithBlockTime(timestamp)
	err := keeper.AddPriceEntry(ctx, source, dataID, price, ctx.BlockTime())
	require.NoError(t, err)

	return ctx
}

func TestGRPCQueryPricesTimestamp(t *testing.T) {
	suite := setupTest(t)

	source := testutil.AccAddress(t)
	params := oracletypes.DefaultParams()
	params.Sources = []string{source.String()}
	params.MinPriceSources = 1
	params.MaxPriceStalenessPeriod = 1000
	params.TwapWindow = 10
	params.MaxPriceDeviationBps = 1000
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	dataID := oracletypes.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD}
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ts1 := baseTime.Add(10 * time.Second)
	ts2 := baseTime.Add(11 * time.Second)

	ctx := suite.ctx
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 10, ts1, sdkmath.LegacyMustNewDecFromStr("1.0"))
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 11, ts2, sdkmath.LegacyMustNewDecFromStr("2.0"))

	// Use pagination-based query to verify prices exist
	allReq := &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
		},
	}
	allRes, err := suite.queryClient.Prices(ctx, allReq)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(allRes.Prices), 2)

	// Query by exact timestamp range (start == end)
	req := &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
			StartTime:  ts1,
			EndTime:    ts1,
		},
	}

	res, err := suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 1)
	require.True(t, res.Prices[0].ID.Timestamp.Equal(ts1))
	require.Equal(t, sdkmath.LegacyMustNewDecFromStr("1.0"), res.Prices[0].State.Price)

	// Query with time range covering both entries
	req = &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
			StartTime:  ts1,
			EndTime:    ts2,
		},
	}

	res, err = suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 2)
}

func TestGRPCQueryPricesDefaultOrder(t *testing.T) {
	suite := setupTest(t)

	source := testutil.AccAddress(t)
	params := oracletypes.DefaultParams()
	params.Sources = []string{source.String()}
	params.MinPriceSources = 1
	params.MaxPriceStalenessPeriod = 1000
	params.TwapWindow = 10
	params.MaxPriceDeviationBps = 1000
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	dataID := oracletypes.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD}
	baseTime := time.Now().UTC().Truncate(time.Nanosecond)

	ts1 := baseTime.Add(10 * time.Second)
	ts2 := baseTime.Add(11 * time.Second)
	ts3 := baseTime.Add(12 * time.Second)

	ctx := suite.ctx
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 10, ts1, sdkmath.LegacyMustNewDecFromStr("1.0"))
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 11, ts2, sdkmath.LegacyMustNewDecFromStr("2.0"))
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 12, ts3, sdkmath.LegacyMustNewDecFromStr("3.0"))

	// Default order (no Reverse flag) should return latest prices first
	req := &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
			StartTime:  baseTime,
			EndTime:    baseTime.Add(time.Minute),
		},
		Pagination: &sdkquery.PageRequest{Limit: 2},
	}

	res, err := suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 2)
	require.NotEmpty(t, res.Pagination.NextKey)
	require.True(t, res.Prices[0].ID.Timestamp.Equal(ts3))
	require.True(t, res.Prices[1].ID.Timestamp.Equal(ts2))

	// Continue with cursor-based pagination
	req = &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
			StartTime:  baseTime,
			EndTime:    baseTime.Add(time.Minute),
		},
		Pagination: &sdkquery.PageRequest{Key: res.Pagination.NextKey, Limit: 2},
	}

	res, err = suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 1)
	require.True(t, res.Prices[0].ID.Timestamp.Equal(ts1))
}

func TestGRPCQueryPricesReverseOrder(t *testing.T) {
	suite := setupTest(t)

	source := testutil.AccAddress(t)
	params := oracletypes.DefaultParams()
	params.Sources = []string{source.String()}
	params.MinPriceSources = 1
	params.MaxPriceStalenessPeriod = 1000
	params.TwapWindow = 10
	params.MaxPriceDeviationBps = 1000
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	dataID := oracletypes.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD}
	baseTime := time.Now().UTC().Truncate(time.Nanosecond)

	ts1 := baseTime.Add(10 * time.Second)
	ts2 := baseTime.Add(11 * time.Second)
	ts3 := baseTime.Add(12 * time.Second)

	ctx := suite.ctx
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 10, ts1, sdkmath.LegacyMustNewDecFromStr("1.0"))
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 11, ts2, sdkmath.LegacyMustNewDecFromStr("2.0"))
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 12, ts3, sdkmath.LegacyMustNewDecFromStr("3.0"))

	// Reverse=true should return oldest prices first
	req := &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
			StartTime:  baseTime,
			EndTime:    baseTime.Add(time.Minute),
		},
		Pagination: &sdkquery.PageRequest{Limit: 2, Reverse: true},
	}

	res, err := suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 2)
	require.NotEmpty(t, res.Pagination.NextKey)
	require.True(t, res.Prices[0].ID.Timestamp.Equal(ts1))
	require.True(t, res.Prices[1].ID.Timestamp.Equal(ts2))

	// Continue with cursor-based pagination
	req = &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
			StartTime:  baseTime,
			EndTime:    baseTime.Add(time.Minute),
		},
		Pagination: &sdkquery.PageRequest{Key: res.Pagination.NextKey, Limit: 2, Reverse: true},
	}

	res, err = suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 1)
	require.True(t, res.Prices[0].ID.Timestamp.Equal(ts3))
}
