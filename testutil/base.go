package testutil

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/rand"

	"github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

// CoinDenom provides ability to create coins in test functions and
// pass them into testutil functionality.
const CoinDenom = "akash"

// Name generates a random name with the given prefix
func Name(_ testing.TB, prefix string) string {
	return fmt.Sprintf("%s-%v", prefix, rand.Uint64())
}

// Hostname generates a random hostname with a "test.com" domain
func Hostname(t testing.TB) string {
	return Name(t, "hostname") + ".test.com"
}

// Attribute generates a random sdk.Attribute
func Attribute(t testing.TB) sdk.Attribute {
	t.Helper()
	return sdk.NewAttribute(Name(t, "attr-key"), Name(t, "attr-value"))
}

// Attributes generates a set of sdk.Attribute
func Attributes(t testing.TB) []sdk.Attribute {
	t.Helper()
	count := rand.Intn(10) + 1

	vals := make([]sdk.Attribute, 0, count)
	for i := 0; i < count; i++ {
		vals = append(vals, Attribute(t))
	}
	return vals

}

// Resources produces an attribute list for populating a Group's
// 'Resources' fields.
func Resources(t testing.TB) []dtypes.Resource {
	t.Helper()
	count := rand.Intn(10) + 1

	vals := make([]dtypes.Resource, 0, count)
	for i := 0; i < count; i++ {
		coin := sdk.NewCoin(CoinDenom, sdk.NewInt(rand.Int63n(9999)))
		res := dtypes.Resource{
			Unit: types.Unit{
				CPU:     100,
				Memory:  100,
				Storage: 10,
			},
			Count: 1,
			Price: coin,
		}
		vals = append(vals, res)
	}
	return vals
}
