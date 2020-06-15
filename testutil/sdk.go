package testutil

import (
	"math/rand"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func Coin(_ testing.TB) sdk.Coin {
	return sdk.NewCoin("testcoin", sdk.NewInt(int64(rand.Intn(999)+1)))
}
