package denom

import (
	"math/big"
	"strconv"
	"strings"
)

// Denom constants
const (
	Mega = 1000000
)

var (
	microSuffixes = []string{
		"mu",
		"µ",
		"u",
	}
)

// ToBase converts a unit of currency to its equivalent value in base denomination
func ToBase(sval string) (uint64, error) {

	for _, suffix := range microSuffixes {
		if !strings.HasSuffix(sval, suffix) {
			continue
		}
		return strconv.ParseUint(strings.TrimSuffix(sval, suffix), 10, 64)
	}

	fval, _, err := big.ParseFloat(sval, 10, 64, big.ToNearestAway)
	if err != nil {
		return 0, err
	}

	mval := new(big.Float).
		Mul(fval, new(big.Float).SetUint64(Mega))

	val, _ := mval.Uint64()
	return val, nil
}
