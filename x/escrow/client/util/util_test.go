package util

import (
	"math"
	"testing"

	sdkmath "cosmossdk.io/math"
)

func TestLeaseCalcBalanceRemain(t *testing.T) {
	tests := []struct {
		name       string
		balance    string
		currBlock  int64
		settledAt  int64
		leasePrice string
		expected   float64
	}{
		{
			name:       "normal case with positive balance remaining",
			balance:    "1000.0",
			currBlock:  100,
			settledAt:  90,
			leasePrice: "10.0",
			expected:   900.0,
		},
		{
			name:       "zero blocks elapsed",
			balance:    "1000.0",
			currBlock:  100,
			settledAt:  100,
			leasePrice: "10.0",
			expected:   1000.0,
		},
		{
			name:       "one block elapsed",
			balance:    "1000.0",
			currBlock:  101,
			settledAt:  100,
			leasePrice: "10.0",
			expected:   990.0,
		},
		{
			name:       "balance depleted exactly",
			balance:    "100.0",
			currBlock:  110,
			settledAt:  100,
			leasePrice: "10.0",
			expected:   0.0,
		},
		{
			name:       "balance overdrafted",
			balance:    "50.0",
			currBlock:  110,
			settledAt:  100,
			leasePrice: "10.0",
			expected:   -50.0,
		},
		{
			name:       "zero balance",
			balance:    "0.0",
			currBlock:  100,
			settledAt:  90,
			leasePrice: "10.0",
			expected:   -100.0,
		},
		{
			name:       "zero lease price",
			balance:    "1000.0",
			currBlock:  100,
			settledAt:  90,
			leasePrice: "0.0",
			expected:   1000.0,
		},
		{
			name:       "large numbers",
			balance:    "1000000.0",
			currBlock:  1000000,
			settledAt:  999000,
			leasePrice: "100.0",
			expected:   900000.0,
		},
		{
			name:       "fractional lease price",
			balance:    "1000.0",
			currBlock:  100,
			settledAt:  90,
			leasePrice: "0.5",
			expected:   995.0,
		},
		{
			name:       "fractional balance",
			balance:    "123.456",
			currBlock:  105,
			settledAt:  100,
			leasePrice: "2.5",
			expected:   110.956,
		},
		{
			name:       "very small lease price",
			balance:    "100.0",
			currBlock:  1000,
			settledAt:  0,
			leasePrice: "0.001",
			expected:   99.0,
		},
		{
			name:       "many blocks elapsed",
			balance:    "10000.0",
			currBlock:  100000,
			settledAt:  0,
			leasePrice: "0.1",
			expected:   0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balance := sdkmath.LegacyMustNewDecFromStr(tt.balance)
			leasePrice := sdkmath.LegacyMustNewDecFromStr(tt.leasePrice)

			result := LeaseCalcBalanceRemain(balance, tt.currBlock, tt.settledAt, leasePrice)

			if !floatEquals(result, tt.expected, 0.0001) {
				t.Errorf("LeaseCalcBalanceRemain() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLeaseCalcBlocksRemain(t *testing.T) {
	tests := []struct {
		name       string
		balance    float64
		leasePrice string
		expected   int64
	}{
		{
			name:       "normal case",
			balance:    1000.0,
			leasePrice: "10.0",
			expected:   100,
		},
		{
			name:       "fractional result rounds down",
			balance:    105.0,
			leasePrice: "10.0",
			expected:   10,
		},
		{
			name:       "zero balance",
			balance:    0.0,
			leasePrice: "10.0",
			expected:   0,
		},
		{
			name:       "small balance with large price",
			balance:    1.0,
			leasePrice: "10.0",
			expected:   0,
		},
		{
			name:       "large balance",
			balance:    1000000.0,
			leasePrice: "0.1",
			expected:   10000000,
		},
		{
			name:       "fractional lease price",
			balance:    100.0,
			leasePrice: "0.5",
			expected:   200,
		},
		{
			name:       "exact division",
			balance:    250.0,
			leasePrice: "2.5",
			expected:   100,
		},
		{
			name:       "very small lease price",
			balance:    100.0,
			leasePrice: "0.001",
			expected:   100000,
		},
		{
			name:       "fractional balance and price",
			balance:    123.456,
			leasePrice: "0.789",
			expected:   156,
		},
		{
			name:       "balance barely covers one block",
			balance:    10.1,
			leasePrice: "10.0",
			expected:   1,
		},
		{
			name:       "balance almost covers one block",
			balance:    9.9,
			leasePrice: "10.0",
			expected:   0,
		},
		{
			name:       "negative balance",
			balance:    -100.0,
			leasePrice: "10.0",
			expected:   -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leasePrice := sdkmath.LegacyMustNewDecFromStr(tt.leasePrice)

			result := LeaseCalcBlocksRemain(tt.balance, leasePrice)

			if result != tt.expected {
				t.Errorf("LeaseCalcBlocksRemain() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLeaseCalcBalanceRemainWithExtremValues(t *testing.T) {
	tests := []struct {
		name       string
		balance    string
		currBlock  int64
		settledAt  int64
		leasePrice string
		validate   func(t *testing.T, result float64)
	}{
		{
			name:       "max int64 blocks",
			balance:    "1000000000.0",
			currBlock:  math.MaxInt64,
			settledAt:  math.MaxInt64 - 1000,
			leasePrice: "1.0",
			validate: func(t *testing.T, result float64) {
				if !floatEquals(result, 999999000.0, 1.0) {
					t.Errorf("LeaseCalcBalanceRemain() = %v, want approximately %v", result, 999999000.0)
				}
			},
		},
		{
			name:       "negative blocks elapsed",
			balance:    "1000.0",
			currBlock:  50,
			settledAt:  100,
			leasePrice: "10.0",
			validate: func(t *testing.T, result float64) {
				if result <= 1000.0 {
					t.Errorf("LeaseCalcBalanceRemain() = %v, should be greater than 1000.0 when currBlock < settledAt", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balance := sdkmath.LegacyMustNewDecFromStr(tt.balance)
			leasePrice := sdkmath.LegacyMustNewDecFromStr(tt.leasePrice)

			result := LeaseCalcBalanceRemain(balance, tt.currBlock, tt.settledAt, leasePrice)

			tt.validate(t, result)
		})
	}
}

func floatEquals(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}
