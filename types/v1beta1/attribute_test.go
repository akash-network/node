package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	types "github.com/akash-network/node/types/v1beta1"
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

func TestAttributes_SubsetOf(t *testing.T) {
	attr1 := types.Attributes{
		{Key: "key1", Value: "val1"},
	}

	attr2 := types.Attributes{
		{Key: "key1", Value: "val1"},
		{Key: "key2", Value: "val2"},
	}

	attr3 := types.Attributes{
		{Key: "key1", Value: "val1"},
		{Key: "key2", Value: "val2"},
		{Key: "key3", Value: "val3"},
		{Key: "key4", Value: "val4"},
	}

	attr4 := types.Attributes{
		{Key: "key3", Value: "val3"},
		{Key: "key4", Value: "val4"},
	}

	require.True(t, attr1.SubsetOf(attr2))
	require.True(t, attr2.SubsetOf(attr3))
	require.False(t, attr1.SubsetOf(attr4))
}
