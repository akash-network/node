package sdl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
)

func TestStorage_LegacyValid(t *testing.T) {
	var stream = `
size: 1Gi
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)

	require.Len(t, p, 1)
	require.Equal(t, byteQuantity(1*unit.Gi), p[0].Quantity)
	require.Len(t, p[0].Attributes, 0)
}

func TestStorage_ArraySingleElemValid(t *testing.T) {
	var stream = `
- size: 1Gi
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)

	require.Len(t, p, 1)
	require.Equal(t, byteQuantity(1*unit.Gi), p[0].Quantity)
	require.Len(t, p[0].Attributes, 0)
}

func TestStorage_AttributesPersistentValidClass(t *testing.T) {
	var stream = `
- size: 1Gi
  attributes:
    persistent: true
    class: default
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)

	require.Len(t, p, 1)
	require.Equal(t, byteQuantity(1*unit.Gi), p[0].Quantity)
	require.Len(t, p[0].Attributes, 2)

	attr := types.Attributes(p[0].Attributes)
	require.Equal(t, attr[0].Key, "class")
	require.Equal(t, attr[0].Value, "default")
}

func TestStorage_AttributesUnknown(t *testing.T) {
	var stream = `
- size: 1Gi
  attributes:
    somefield: foo
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.ErrorIs(t, err, errUnsupportedStorageAttribute)
}

func TestStorage_MultipleUnnamedEphemeral(t *testing.T) {
	var stream = `
- size: 1Gi
- size: 2Gi
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.EqualError(t, err, errStorageDuplicatedVolumeName.Error())
}

func TestStorage_EphemeralNoClass(t *testing.T) {
	var stream = `
- size: 1Gi
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
}

func TestStorage_EphemeralClass(t *testing.T) {
	var stream = `
- size: 1Gi
  attributes:
    class: foo
`

	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.EqualError(t, err, errStorageEphemeralClass.Error())
}

func TestStorage_PersistentDefaultClass(t *testing.T) {
	var stream = `
- size: 1Gi
  attributes:
    persistent: true
`

	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Len(t, p[0].Attributes, 2)

	require.Equal(t, p[0].Attributes[0].Key, "class")
	require.Equal(t, p[0].Attributes[0].Value, "default")
}

func TestStorage_PersistentClass(t *testing.T) {
	var stream = `
- size: 1Gi
  attributes:
    persistent: true
    class: beta1
`

	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Len(t, p[0].Attributes, 2)

	require.Equal(t, p[0].Attributes[0].Key, "class")
	require.Equal(t, p[0].Attributes[0].Value, "beta1")
}

func TestStorage_StableSort(t *testing.T) {
	storage := v2ResourceStorageArray{
		{
			Quantity: 2 * unit.Gi,
			Attributes: v2StorageAttributes{
				types.Attribute{
					Key:   "persistent",
					Value: "true",
				},
			},
		},
		{
			Quantity: 1 * unit.Gi,
		},
		{
			Quantity: 10 * unit.Gi,
		},
	}

	storage.sort()

	require.Equal(t, byteQuantity(1*unit.Gi), storage[0].Quantity)
	require.Equal(t, byteQuantity(2*unit.Gi), storage[1].Quantity)
	require.Equal(t, byteQuantity(10*unit.Gi), storage[2].Quantity)
}

func TestStorage_Invalid_InvalidMount(t *testing.T) {
	_, err := ReadFile("./_testdata/storageClass1.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected absolute path")
}

func TestStorage_Invalid_MountNotAbsolute(t *testing.T) {
	_, err := ReadFile("./_testdata/storageClass2.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected absolute path")
}

func TestStorage_Invalid_VolumeReference(t *testing.T) {
	_, err := ReadFile("./_testdata/storageClass3.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "references to no-existing compute volume")
}

func TestStorage_Invalid_DuplicatedMount(t *testing.T) {
	_, err := ReadFile("./_testdata/storageClass4.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already in use by volume")
}

func TestStorage_Invalid_NoMount(t *testing.T) {
	_, err := ReadFile("./_testdata/storageClass5.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "to have mount")
}
