package v1beta1

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

func TestResourceValueSubIsIdempotent(t *testing.T) {
	val1 := NewResourceValue(100)
	before := val1.String()
	val2 := NewResourceValue(1)

	_, err := val1.sub(val2)
	require.NoError(t, err)
	after := val1.String()

	require.Equal(t, before, after)
}

func TestCPUSubIsNotIdempotent(t *testing.T) {
	val1 := &CPU{
		Units:      NewResourceValue(100),
		Attributes: nil,
	}

	before := val1.String()
	val2 := &CPU{
		Units:      NewResourceValue(1),
		Attributes: nil,
	}

	err := val1.sub(val2)
	require.NoError(t, err)
	after := val1.String()

	require.NotEqual(t, before, after)
}
