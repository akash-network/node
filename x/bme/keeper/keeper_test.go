package keeper

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/collections"

	types "pkg.akt.dev/go/node/bme/v1"
)

func defaultTestParams() types.Params {
	return types.Params{
		CircuitBreakerWarnThreshold: 9500, // 0.95
		CircuitBreakerHaltThreshold: 9000, // 0.90
		MinEpochBlocks:              10,
		EpochBlocksBackoffPercent:   10, // 10%
	}
}

func TestCalculateBlocksDiff_AtOrAboveWarnThreshold(t *testing.T) {
	params := defaultTestParams()

	// Exactly at warn threshold
	require.Equal(t, params.MinEpochBlocks, calculateBlocksDiff(params, 9500))
	// Above warn threshold
	require.Equal(t, params.MinEpochBlocks, calculateBlocksDiff(params, 9600))
	require.Equal(t, params.MinEpochBlocks, calculateBlocksDiff(params, 10000))
}

func TestCalculateBlocksDiff_ZeroBackoff(t *testing.T) {
	params := defaultTestParams()
	params.EpochBlocksBackoffPercent = 0

	// Even with CR well below warn threshold, should return MinEpochBlocks
	require.Equal(t, params.MinEpochBlocks, calculateBlocksDiff(params, 9200))
	require.Equal(t, params.MinEpochBlocks, calculateBlocksDiff(params, 9000))
}

func TestCalculateBlocksDiff_StepsTooSmall(t *testing.T) {
	params := defaultTestParams()

	// CR = 9495: drop = 5, steps = 5/10 = 0 (integer division)
	require.Equal(t, params.MinEpochBlocks, calculateBlocksDiff(params, 9495))
	// CR = 9491: drop = 9, steps = 9/10 = 0
	require.Equal(t, params.MinEpochBlocks, calculateBlocksDiff(params, 9491))
}

func TestCalculateBlocksDiff_DefaultParams(t *testing.T) {
	// EpochBlocksBackoffPercent=10 (10%), base = 1.1
	// result = 10 * 1.1^steps, truncated
	params := defaultTestParams()

	tests := []struct {
		name     string
		cr       int64
		expected int64
	}{
		// 0.950 → steps=0 → 10
		{"cr_9500_steps_0", 9500, 10},
		// 0.949 → steps=1 → floor(10 * 1.1) = 11
		{"cr_9490_steps_1", 9490, 11},
		// 0.948 → steps=2 → floor(10 * 1.21) = 12
		{"cr_9480_steps_2", 9480, 12},
		// 0.945 → steps=5 → floor(10 * 1.1^5) = floor(16.1051) = 16
		{"cr_9450_steps_5", 9450, 16},
		// 0.940 → steps=10 → floor(10 * 1.1^10) = floor(25.937) = 25
		{"cr_9400_steps_10", 9400, 25},
		// 0.930 → steps=20 → floor(10 * 1.1^20) = floor(67.275) = 67
		{"cr_9300_steps_20", 9300, 67},
		// 0.920 → steps=30 → floor(10 * 1.1^30) = floor(174.494) = 174
		{"cr_9200_steps_30", 9200, 174},
		// 0.910 → steps=40 → floor(10 * 1.1^40) = floor(452.593) = 452
		{"cr_9100_steps_40", 9100, 452},
		// 0.900 → steps=50 → floor(10 * 1.1^50) = floor(1173.909) = 1173
		{"cr_9000_steps_50", 9000, 1173},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateBlocksDiff(params, tc.cr)
			require.Equal(t, tc.expected, result, "cr=%d", tc.cr)
		})
	}
}

func TestCalculateBlocksDiff_AggressiveBackoff(t *testing.T) {
	// EpochBlocksBackoffPercent=100 (100%), base = 2.0
	// result = 10 * 2^steps, truncated
	params := defaultTestParams()
	params.EpochBlocksBackoffPercent = 100

	tests := []struct {
		name     string
		cr       int64
		expected int64
	}{
		{"cr_9500_steps_0", 9500, 10},
		// steps=1 → 10 * 2 = 20
		{"cr_9490_steps_1", 9490, 20},
		// steps=2 → 10 * 4 = 40
		{"cr_9480_steps_2", 9480, 40},
		// steps=5 → 10 * 32 = 320
		{"cr_9450_steps_5", 9450, 320},
		// steps=10 → 10 * 1024 = 10240
		{"cr_9400_steps_10", 9400, 10240},
		// steps=11 → 10 * 2048 = 20480 → capped at 14400
		{"cr_9390_steps_11_capped", 9390, 14400},
		// steps=50 → massive number → capped at 14400
		{"cr_9000_steps_50_capped", 9000, 14400},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateBlocksDiff(params, tc.cr)
			require.Equal(t, tc.expected, result, "cr=%d", tc.cr)
		})
	}
}

func TestCalculateBlocksDiff_Cap14400(t *testing.T) {
	params := defaultTestParams()
	params.EpochBlocksBackoffPercent = 50 // 50%, base = 1.5

	// steps=20 → floor(10 * 1.5^20) = floor(33252.5...) → capped at 14400
	result := calculateBlocksDiff(params, 9300)
	require.Equal(t, int64(14400), result)
}

func TestCalculateBlocksDiff_NoOverflowPanic(t *testing.T) {
	// Ensure no panic with extreme parameters that would cause
	// int64 overflow if not for the Dec cap.
	params := defaultTestParams()
	params.EpochBlocksBackoffPercent = 100 // 100%, base = 2.0

	// steps=50 → 2^50 * 10 = ~1.13 * 10^16, well beyond int64
	// Should not panic, should return 14400
	require.NotPanics(t, func() {
		result := calculateBlocksDiff(params, 9000)
		require.Equal(t, int64(14400), result)
	})
}

func TestCalculateBlocksDiff_BelowHaltThreshold(t *testing.T) {
	// CR below halt threshold is still valid input—calculateBlocksDiff
	// doesn't enforce halt, it just computes the backoff.
	params := defaultTestParams()

	// cr=8900, steps=(9500-8900)/10=60 → floor(10 * 1.1^60) = floor(3044.8) = 3044
	result := calculateBlocksDiff(params, 8900)
	require.Equal(t, int64(3044), result)
}

func TestCalculateBlocksDiff_MinEpochBlocksFloor(t *testing.T) {
	params := defaultTestParams()
	params.MinEpochBlocks = 100

	// At warn threshold, should return MinEpochBlocks regardless of value
	require.Equal(t, int64(100), calculateBlocksDiff(params, 9500))

	// steps=1 → floor(100 * 1.1) = 110
	require.Equal(t, int64(110), calculateBlocksDiff(params, 9490))
}

func TestCalculateBlocksDiff_SmallBackoff(t *testing.T) {
	// EpochBlocksBackoffPercent=1 (1%), base = 1.01
	params := defaultTestParams()
	params.EpochBlocksBackoffPercent = 1

	// steps=1 → floor(10 * 1.01) = 10
	require.Equal(t, int64(10), calculateBlocksDiff(params, 9490))
	// steps=10 → floor(10 * 1.01^10) = floor(11.046) = 11
	require.Equal(t, int64(11), calculateBlocksDiff(params, 9400))
	// steps=50 → floor(10 * 1.01^50) = floor(16.446) = 16
	require.Equal(t, int64(16), calculateBlocksDiff(params, 9000))
}

// --- Security tests ---

func TestCalculateBlocksDiff_CRZero(t *testing.T) {
	// CR=0 means maximum steps. Must not panic, should cap at 14400.
	params := defaultTestParams()

	require.NotPanics(t, func() {
		result := calculateBlocksDiff(params, 0)
		// steps = (9500-0)/10 = 950, 1.1^950 is astronomical → capped
		require.Equal(t, int64(14400), result)
	})
}

func TestCalculateBlocksDiff_MaxSteps(t *testing.T) {
	// Wide threshold gap: warn=10000, halt=0
	params := defaultTestParams()
	params.CircuitBreakerWarnThreshold = 10000
	params.CircuitBreakerHaltThreshold = 0

	// cr=0 → steps=1000, extreme exponent → must cap at 14400
	require.NotPanics(t, func() {
		result := calculateBlocksDiff(params, 0)
		require.Equal(t, int64(14400), result)
	})
}

func TestCalculateBlocksDiff_LargeCRValue(t *testing.T) {
	// CR clamped to math.MaxUint32 by mintStatusUpdate before calling
	// calculateBlocksDiff. Verify it handles large int64 values gracefully.
	params := defaultTestParams()

	result := calculateBlocksDiff(params, math.MaxUint32)
	// math.MaxUint32 > 9500 (warn threshold), so should return MinEpochBlocks
	require.Equal(t, params.MinEpochBlocks, result)
}

func TestStoragePrefixesUnique(t *testing.T) {
	// V6: All storage key prefixes must be unique to prevent state corruption.
	prefixes := map[string]collections.Prefix{
		"RemintCredits":     RemintCreditsKey,
		"TotalBurned":       TotalBurnedKey,
		"TotalMinted":       TotalMintedKey,
		"LedgerPending":     LedgerPendingKey,
		"Ledger":            LedgerKey,
		"MintStatus":        MintStatusKey,
		"MintEpoch":         MintEpochKey,
		"MintStatusRecords": MintStatusRecordsKey,
		"Params":            ParamsKey,
	}

	seen := make(map[string]string) // prefix string → key name
	for name, p := range prefixes {
		key := fmt.Sprintf("%v", p)
		if existing, ok := seen[key]; ok {
			t.Fatalf("prefix collision: %s and %s share prefix %s", existing, name, key)
		}
		seen[key] = name
	}
}
