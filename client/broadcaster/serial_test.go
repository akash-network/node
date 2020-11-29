package broadcaster

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseNextSequence(t *testing.T) {
	const (
		rawlog = "account sequence mismatch, expected 25, got 27: incorrect account sequence"
	)

	const (
		expected uint64 = 25
		current  uint64 = 27
	)

	nextseq, ok := parseNextSequence(current, rawlog)
	assert.True(t, ok)

	assert.Equal(t, expected, nextseq)

}
