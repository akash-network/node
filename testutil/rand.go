package testutil

import (
	"math/rand"
	"time"
)

// non-constant random seed for math/rand functions

func init() {
	rand.Seed(time.Now().Unix())
}
