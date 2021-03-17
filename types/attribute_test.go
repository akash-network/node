package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ovrclk/akash/types"
)

func TestAttributes_Validate(t *testing.T) {
	attr := types.Attributes{
		{Key: "key"},
		{Key: "key"},
	}

	require.EqualError(t, attr.Validate(), types.ErrAttributesDuplicateKeys.Error())

	// unsupported key symbol
	attr = types.Attributes{
		{Key: "$"},
	}

	require.EqualError(t, attr.Validate(), types.ErrInvalidAttributeKey.Error())

	// empty key
	attr = types.Attributes{
		{Key: ""},
	}

	require.EqualError(t, attr.Validate(), types.ErrInvalidAttributeKey.Error())
	// to long key
	attr = types.Attributes{
		{Key: "sdgkhaeirugaeroigheirghseiargfs3s"},
	}

	require.EqualError(t, attr.Validate(), types.ErrInvalidAttributeKey.Error())
}

func TestAttribute_Equal(t *testing.T) {
	attr1 := &types.Attribute{Key: "key1", Value: "val1"}
	attr2 := &types.Attribute{Key: "key1", Value: "val1"}
	attr3 := &types.Attribute{Key: "key1", Value: "val2"}

	require.True(t, attr1.Equal(attr2))
	require.False(t, attr1.Equal(attr3))
}

func TestAttribute_SubsetOf(t *testing.T) {
	attr1 := types.Attribute{Key: "key1", Value: "val1"}
	attr2 := types.Attribute{Key: "key1", Value: "val1"}
	attr3 := types.Attribute{Key: "key1", Value: "val2"}

	require.True(t, attr1.SubsetOf(attr2))
	require.False(t, attr1.SubsetOf(attr3))
}
