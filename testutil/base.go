package testutil

import (
	"fmt"
	"math/rand"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

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
