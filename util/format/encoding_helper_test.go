package format

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatFixedLengthU64(t *testing.T) {
	tests := map[string]struct {
		d    uint64
		want string
	}{
		"0":       {0, "00000000000000000000"},
		"1":       {1, "00000000000000000001"},
		"9":       {9, "00000000000000000009"},
		"10":      {10, "00000000000000000010"},
		"123":     {123, "00000000000000000123"},
		"max u64": {math.MaxUint64, "18446744073709551615"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := FormatFixedLengthU64(tt.d)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, len(got), 20)
		})
	}
}
