package testutil

import (
	"math/rand"
	"testing"

	"github.com/ovrclk/akash/types"
)

func Unit(_ testing.TB) types.Unit {
	return types.Unit{
		CPU:     uint32(rand.Intn(999) + 1),
		Memory:  uint64(rand.Intn(999) + 1),
		Storage: uint64(rand.Intn(999) + 1),
	}
}
