//nolint: revive

package util

import (
	sdkmath "cosmossdk.io/math"
)

func LeaseCalcBalanceRemain(balance sdkmath.LegacyDec, currBlock, settledAt int64, leasePrice sdkmath.LegacyDec) float64 {
	return balance.MustFloat64() - (float64(currBlock-settledAt))*leasePrice.MustFloat64()
}

func LeaseCalcBlocksRemain(balance float64, leasePrice sdkmath.LegacyDec) int64 {
	return int64(balance / leasePrice.MustFloat64())
}
