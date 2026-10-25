package keeper

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/oracle/v2"
)

func pd(ts time.Time, price string) types.PriceData {
	return types.PriceData{
		ID:    types.PriceDataRecordID{Timestamp: ts},
		State: types.PriceDataState{Price: sdkmath.LegacyMustNewDecFromStr(price)},
	}
}

func TestCalculateMedian(t *testing.T) {
	t.Run("empty returns zero", func(t *testing.T) {
		require.True(t, calculateMedian(nil).IsZero())
	})

	t.Run("odd count returns middle value", func(t *testing.T) {
		now := time.Now()
		// deliberately unsorted input to exercise the internal sort
		in := []types.PriceData{pd(now, "30"), pd(now, "10"), pd(now, "20")}
		require.True(t, sdkmath.LegacyNewDec(20).Equal(calculateMedian(in)),
			"median of {10,20,30} must be 20, got %s", calculateMedian(in))
	})

	t.Run("even count averages the two middle values", func(t *testing.T) {
		now := time.Now()
		in := []types.PriceData{pd(now, "10"), pd(now, "40"), pd(now, "20"), pd(now, "30")}
		// sorted: 10,20,30,40 -> (20+30)/2 = 25
		require.True(t, sdkmath.LegacyNewDec(25).Equal(calculateMedian(in)),
			"median of {10,20,30,40} must be 25, got %s", calculateMedian(in))
	})
}

func TestCalculateDeviationBps(t *testing.T) {
	t.Run("zero min returns zero", func(t *testing.T) {
		require.Equal(t, uint64(0),
			calculateDeviationBps(sdkmath.LegacyZeroDec(), sdkmath.LegacyNewDec(100)))
	})

	t.Run("computes (max-min)/min in basis points", func(t *testing.T) {
		// (110-100)/100 * 10000 = 1000 bps
		require.Equal(t, uint64(1000),
			calculateDeviationBps(sdkmath.LegacyNewDec(100), sdkmath.LegacyNewDec(110)))
	})

	t.Run("no deviation returns zero", func(t *testing.T) {
		require.Equal(t, uint64(0),
			calculateDeviationBps(sdkmath.LegacyNewDec(100), sdkmath.LegacyNewDec(100)))
	})
}

func TestCalculateTWAP(t *testing.T) {
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty returns error", func(t *testing.T) {
		_, err := calculateTWAP(base, nil)
		require.Error(t, err)
	})

	t.Run("single point weighted by elapsed time returns that price", func(t *testing.T) {
		now := base.Add(30 * time.Second)
		in := []types.PriceData{pd(base.Add(20*time.Second), "100")}
		twap, err := calculateTWAP(now, in)
		require.NoError(t, err)
		require.True(t, sdkmath.LegacyNewDec(100).Equal(twap), "got %s", twap)
	})

	t.Run("descending points weighted by time each price prevails", func(t *testing.T) {
		// now = :30
		// newest  @ :20  price 100 -> weight (now - :20)      = 10s
		// older   @ :00  price 220 -> weight (:20 - :00)       = 20s
		// TWAP = (100*10 + 220*20) / 30 = 5400 / 30 = 180
		now := base.Add(30 * time.Second)
		in := []types.PriceData{
			pd(base.Add(20*time.Second), "100"),
			pd(base, "220"),
		}
		twap, err := calculateTWAP(now, in)
		require.NoError(t, err)
		require.True(t, sdkmath.LegacyNewDec(180).Equal(twap),
			"time-weighted average must be 180, got %s", twap)
	})

	t.Run("identical timestamps fall back to simple average", func(t *testing.T) {
		// all points share the block time -> totalWeight == 0 -> simple average
		now := base
		in := []types.PriceData{pd(base, "100"), pd(base, "200")}
		twap, err := calculateTWAP(now, in)
		require.NoError(t, err)
		require.True(t, sdkmath.LegacyNewDec(150).Equal(twap),
			"fallback average of {100,200} must be 150, got %s", twap)
	})
}
