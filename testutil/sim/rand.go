package sim

import (
	"math/rand"
)

func RandIdx(r *rand.Rand, val int) int {
	if val == 0 {
		return 0
	}

	return r.Intn(val)
}
