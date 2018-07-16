package denom

import (
	"strconv"
)

const (
	miliAkashPerAkash = 1000
)

// ToBase converts a unit of currency to its equivalent value in base denomination
func ToBase(sval string) (uint64, error) {
	akash, err := strconv.ParseFloat(sval, 64)
	if err != nil {
		return 0, err
	}
	amount := akash * miliAkashPerAkash
	return uint64(amount), nil
}
