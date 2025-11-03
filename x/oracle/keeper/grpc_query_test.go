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

	oracletypes "pkg.akt.dev/go/node/oracle/v1"
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
	err := keeper.AddPriceEntry(ctx, source, dataID, oracletypes.PriceDataState{
		Price:     price,
		Timestamp: ctx.BlockTime(),
	})
	require.NoError(t, err)

	return ctx
}

func TestGRPCQueryPricesHeight(t *testing.T) {
	suite := setupTest(t)

	source := testutil.AccAddress(t)
	params := oracletypes.Params{
		Sources:                 []string{source.String()},
		MinPriceSources:         1,
		MaxPriceStalenessBlocks: 1000,
		TwapWindow:              10,
		MaxPriceDeviationBps:    1000,
	}
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	dataID := oracletypes.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD}
	baseTime := time.Now().UTC()

	ctx := suite.ctx
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 10, baseTime.Add(10*time.Second), sdkmath.LegacyMustNewDecFromStr("1.0"))
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 11, baseTime.Add(11*time.Second), sdkmath.LegacyMustNewDecFromStr("2.0"))

	req := &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
			Height:     10,
		},
	}

	res, err := suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 1)
	require.Nil(t, res.Pagination)
	require.Equal(t, int64(10), res.Prices[0].ID.Height)
	require.Equal(t, sdkmath.LegacyMustNewDecFromStr("1.0"), res.Prices[0].State.Price)
}

func TestGRPCQueryPricesPaginationReverse(t *testing.T) {
	suite := setupTest(t)

	source := testutil.AccAddress(t)
	params := oracletypes.Params{
		Sources:                 []string{source.String()},
		MinPriceSources:         1,
		MaxPriceStalenessBlocks: 1000,
		TwapWindow:              10,
		MaxPriceDeviationBps:    1000,
	}
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	dataID := oracletypes.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD}
	baseTime := time.Now().UTC()

	ctx := suite.ctx
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 10, baseTime.Add(10*time.Second), sdkmath.LegacyMustNewDecFromStr("1.0"))
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 11, baseTime.Add(11*time.Second), sdkmath.LegacyMustNewDecFromStr("2.0"))
	ctx = addPriceEntry(t, ctx, suite.keeper, source, dataID, 12, baseTime.Add(12*time.Second), sdkmath.LegacyMustNewDecFromStr("3.0"))

	req := &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
		},
		Pagination: &sdkquery.PageRequest{Limit: 2},
	}

	res, err := suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 2)
	require.NotEmpty(t, res.Pagination.NextKey)
	require.Equal(t, int64(12), res.Prices[0].ID.Height)
	require.Equal(t, int64(11), res.Prices[1].ID.Height)

	req = &oracletypes.QueryPricesRequest{
		Filters: oracletypes.PricesFilter{
			AssetDenom: sdkutil.DenomAkt,
			BaseDenom:  sdkutil.DenomUSD,
		},
		Pagination: &sdkquery.PageRequest{Key: res.Pagination.NextKey, Limit: 2},
	}

	res, err = suite.queryClient.Prices(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Prices, 2)
	require.Equal(t, int64(10), res.Prices[0].ID.Height)
	require.Equal(t, int64(0), res.Prices[1].ID.Height)
}
