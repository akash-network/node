package denom_test

import (
	"testing"

	"github.com/ovrclk/akash/denom"
	"github.com/stretchr/testify/assert"
)

func TestToBase(t *testing.T) {

	tests := []struct {
		sval string
		val  uint64
		ok   bool
	}{
		{"1", 1 * denom.Mega, true},
		{"100", 100 * denom.Mega, true},
		{"0.000001", 1, true},
		{"0.000100", 100, true},
		{"1.000001", denom.Mega + 1, true},
		{"0.0000001", 0, true},
		{"1e-6", 1, true},
		{"3e2", 300 * denom.Mega, true},

		{"100u", 100, true},
		{"35µ", 35, true},
		{"200mu", 200, true},

		{"0x100", 0, false},
		{"abcd", 0, false},
		{"u", 0, false},
		{"fu", 0, false},
	}

	for _, test := range tests {
		val, err := denom.ToBase(test.sval)
		if test.ok {
			assert.Equal(t, test.val, val, test.sval)
			continue
		}
		assert.Error(t, err, test.sval)
	}
}
