package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidSum(t *testing.T) {
	val1 := NewResourceValue(1)
	val2 := NewResourceValue(1)

	res, err := val1.add(val2)
	require.NoError(t, err)
	require.Equal(t, uint64(2), res.Value())
}

func TestSubToNegative(t *testing.T) {
	val1 := NewResourceValue(1)
	val2 := NewResourceValue(2)

	_, err := val1.sub(val2)
	require.Error(t, err)
}
