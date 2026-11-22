package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/testutil"

	oracletypes "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"
)

// TestEndBlockerExtremeDeviationDoesNotHalt reproduces the EndBlocker chain-halt:
// an authorized source reporting a near-zero price against a normal one drives
// the price deviation past uint64, which previously panicked in
// calculateDeviationBps (Uint64() out of bounds) inside EndBlocker and halted the
// chain. EndBlocker must not panic, and the overflowed (saturated) deviation must
// leave the price unhealthy, so GetAggregatedPrice reports it as unavailable.
func TestEndBlockerExtremeDeviationDoesNotHalt(t *testing.T) {
	suite := setupTest(t)

	s1 := testutil.AccAddress(t)
	s2 := testutil.AccAddress(t)

	params := oracletypes.DefaultParams()
	params.Sources = []string{s1.String(), s2.String()}
	params.MinPriceSources = 1
	params.MaxPriceStalenessPeriod = time.Hour
	params.TwapWindow = time.Hour
	params.MaxPriceDeviationBps = 1000
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	dataID := oracletypes.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ts1 := base.Add(10 * time.Second)
	ts2 := base.Add(11 * time.Second)

	ctx := suite.ctx
	ctx = addPriceEntry(t, ctx, suite.keeper, s1, dataID, 10, ts1, sdkmath.LegacyMustNewDecFromStr("100"))
	ctx = addPriceEntry(t, ctx, suite.keeper, s2, dataID, 11, ts2, sdkmath.LegacyMustNewDecFromStr("0.000000000000000001"))

	require.NotPanics(t, func() {
		require.NoError(t, suite.keeper.EndBlocker(ctx))
	})

	_, err := suite.keeper.GetAggregatedPrice(ctx, sdkutil.DenomAkt)
	require.Error(t, err, "an overflowed (saturated) deviation must be unhealthy, so no aggregate is available")
}

// TestEndBlockerNormalDeviationHealthy is the control: two in-range prices from
// two sources produce a healthy, stored aggregate. This confirms the harness
// actually drives the multi-source aggregation path (so the test above is not
// vacuous) and that the fix leaves normal deviations untouched.
func TestEndBlockerNormalDeviationHealthy(t *testing.T) {
	suite := setupTest(t)

	s1 := testutil.AccAddress(t)
	s2 := testutil.AccAddress(t)

	params := oracletypes.DefaultParams()
	params.Sources = []string{s1.String(), s2.String()}
	params.MinPriceSources = 1
	params.MaxPriceStalenessPeriod = time.Hour
	params.TwapWindow = time.Hour
	params.MaxPriceDeviationBps = 1000
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	dataID := oracletypes.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ts1 := base.Add(10 * time.Second)
	ts2 := base.Add(11 * time.Second)

	ctx := suite.ctx
	ctx = addPriceEntry(t, ctx, suite.keeper, s1, dataID, 10, ts1, sdkmath.LegacyMustNewDecFromStr("100"))
	ctx = addPriceEntry(t, ctx, suite.keeper, s2, dataID, 11, ts2, sdkmath.LegacyMustNewDecFromStr("101"))

	require.NotPanics(t, func() {
		require.NoError(t, suite.keeper.EndBlocker(ctx))
	})

	price, err := suite.keeper.GetAggregatedPrice(ctx, sdkutil.DenomAkt)
	require.NoError(t, err, "a normal deviation must be healthy and available")
	require.True(t, price.IsPositive())
}
