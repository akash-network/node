package util

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func LeaseCalcBalanceRemain(balance sdk.Dec, currBlock, settledAt int64, leasePrice sdk.Dec) float64 {
	return balance.MustFloat64() - (float64(currBlock-settledAt))*leasePrice.MustFloat64()
}

func LeaseCalcBlocksRemain(balance float64, leasePrice sdk.Dec) int64 {
	return int64(balance / leasePrice.MustFloat64())
}
