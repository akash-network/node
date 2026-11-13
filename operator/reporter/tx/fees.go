package tx

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// FeeFor returns fees when set, otherwise ceil(gasLimit × price) for each of prices.
func FeeFor(gasLimit uint64, prices sdk.DecCoins, fees sdk.Coins) sdk.Coins {
	if !fees.IsZero() {
		return fees
	}

	gas := sdkmath.LegacyNewDecFromInt(sdkmath.NewIntFromUint64(gasLimit))
	out := make([]sdk.Coin, 0, len(prices))

	for _, price := range prices {
		out = append(out, sdk.NewCoin(price.Denom, price.Amount.Mul(gas).Ceil().TruncateInt()))
	}

	return sdk.NewCoins(out...)
}
