package denom

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	miliAkash = 1
	Akash     = 1000 * miliAkash
)

var unitSuffixes = []struct {
	symbol string
	unit   uint64
}{
	{"A", Akash},
	{"m", miliAkash},
}

// ToBase converts a unit of currency to its equivalent value in base denomination
func ToBase(sval string) (uint64, error) {
	for _, suffix := range unitSuffixes {
		if !strings.HasSuffix(sval, suffix.symbol) {
			continue
		}
		sval := strings.TrimSuffix(sval, suffix.symbol)
		val, err := strconv.ParseUint(sval, 10, 64)
		if err != nil {
			return 0, err
		}
		amount := val * suffix.unit
		return amount, nil
	}

	return 0, fmt.Errorf("unrecognized denomination %s", sval)
}
