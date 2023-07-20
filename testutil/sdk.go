package testutil

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func Coin(t testing.TB) sdk.Coin {
	t.Helper()
	return sdk.NewCoin("testcoin", sdk.NewInt(int64(RandRangeInt(1, 1000)))) // nolint: gosec
}

func DecCoin(t testing.TB) sdk.DecCoin {
	t.Helper()
	return sdk.NewDecCoin("testcoin", sdk.NewInt(int64(RandRangeInt(1, 1000)))) // nolint: gosec
}

// AkashCoinRandom provides simple interface to the Akash sdk.Coin type.
func AkashCoinRandom(t testing.TB) sdk.Coin {
	t.Helper()
	amt := sdk.NewInt(int64(RandRangeInt(1, 1000)))
	return sdk.NewCoin(CoinDenom, amt)
}

// AkashCoin provides simple interface to the Akash sdk.Coin type.
func AkashCoin(t testing.TB, amount int64) sdk.Coin {
	t.Helper()
	amt := sdk.NewInt(amount)
	return sdk.NewCoin(CoinDenom, amt)
}

func AkashDecCoin(t testing.TB, amount int64) sdk.DecCoin {
	t.Helper()
	amt := sdk.NewInt(amount)
	return sdk.NewDecCoin(CoinDenom, amt)
}

func AkashDecCoinRandom(t testing.TB) sdk.DecCoin {
	t.Helper()
	amt := sdk.NewInt(int64(RandRangeInt(1, 1000)))
	return sdk.NewDecCoin(CoinDenom, amt)
}
