package testutil

import (
	"math/rand"
	"testing"

	"github.com/ovrclk/akash/types"
)

func Unit(_ testing.TB) types.Unit {
	return types.Unit{
		CPU:     rand.Uint32(),
		Memory:  rand.Uint64(),
		Storage: rand.Uint64(),
	}
}
