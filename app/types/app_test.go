package types //nolint: revive

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindStructFieldEmptyField(t *testing.T) {
	type testType struct {
		Val string
	}
	testStruct := testType{}

	_, err := FindStructField[string](testStruct, "")
	require.EqualError(t, err, ErrEmptyFieldName.Error())
}

func TestFindStructFieldObjectAsValue(t *testing.T) {
	type testType struct {
		Val string
	}
	testStruct := testType{}

	val, err := FindStructField[string](testStruct, "Val")
	require.NoError(t, err)
	require.Equal(t, "", val)
}

func TestFindStructFieldObjectAsPointer(t *testing.T) {
	type testType struct {
		Val string
	}
	testStruct := testType{
		Val: "testVal",
	}

	val, err := FindStructField[string](&testStruct, "Val")
	require.NoError(t, err)
	require.Equal(t, "testVal", val)
}

func TestFindStructFieldUnknownField(t *testing.T) {
	type testType struct {
		Val string
	}
	testStruct := testType{
		Val: "testVal",
	}

	_, err := FindStructField[string](&testStruct, "Vals")
	require.Error(t, err)
}

func TestFindStructFieldNonMatchingType(t *testing.T) {
	type testType struct {
		Val string
	}
	testStruct := testType{
		Val: "testVal",
	}

	_, err := FindStructField[int](&testStruct, "Vals")
	require.Error(t, err)
}
