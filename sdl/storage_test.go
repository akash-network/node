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

func TestStorage_AttributesValidClass(t *testing.T) {
	var stream = `
- size: 1Gi
  attributes:
    class: ssd
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)

	require.Len(t, p, 1)
	require.Equal(t, byteQuantity(1*unit.Gi), p[0].Quantity)
	require.Len(t, p[0].Attributes, 1)

	attr := types.Attributes(p[0].Attributes)
	require.Equal(t, attr[0].Key, "class")
	require.Equal(t, attr[0].Value, "ssd")
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

func TestStorage_AttributesPersistentInvalid(t *testing.T) {
	var stream = `
- size: 1Gi
  attributes:
    persistent: true
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.ErrorIs(t, err, errStorageMountPoint)
}

// func TestStorage_AttributesPersistentValid(t *testing.T) {
// 	var stream = `
// - size: 1Gi
//   attributes:
//     persistent: true
//     mount: /var/lib/foo
// `
// 	var p v2ResourceStorageArray
//
// 	err := yaml.Unmarshal([]byte(stream), &p)
// 	require.NoError(t, err)
// }

// func TestStorage_AttributesMountValid(t *testing.T) {
// 	var stream = `
// - size: 1Gi
//   attributes:
//     mount: /var/lib/foo
// `
// 	var p v2ResourceStorageArray
//
// 	err := yaml.Unmarshal([]byte(stream), &p)
// 	require.NoError(t, err)
// }

func TestStorage_MultipleEphemeral(t *testing.T) {
	var stream = `
- size: 1Gi
- size: 2Gi
`
	var p v2ResourceStorageArray

	err := yaml.Unmarshal([]byte(stream), &p)
	require.EqualError(t, err, errStorageMultipleEphemeral.Error())
}

// func TestStorage_MultipleEphemeralValid(t *testing.T) {
// 	var stream = `
// - size: 1Gi
// - size: 2Gi
//   attributes:
//     mount: /var/log
// `
// 	var p v2ResourceStorageArray
//
// 	err := yaml.Unmarshal([]byte(stream), &p)
// 	require.NoError(t, err)
// }
//
// func TestStorage_DuplicatedMount(t *testing.T) {
// 	var stream = `
// - size: 1Gi
//   attributes:
//     mount: /var/log
// - size: 2Gi
//   attributes:
//     mount: /var/log
// `
// 	var p v2ResourceStorageArray
//
// 	err := yaml.Unmarshal([]byte(stream), &p)
// 	require.ErrorIs(t, err, errStorageDupMountPoint)
// }

func TestStorage_StableSort1(t *testing.T) {
	storage := v2ResourceStorageArray{
		{
			Quantity: 2 * unit.Gi,
			Attributes: v2StorageAttributes{
				// types.Attribute{
				// 	Key:   "mount",
				// 	Value: "/usr/local/mongod/data",
				// },
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

func TestStorage_StableSortSameSize(t *testing.T) {
	storage := v2ResourceStorageArray{
		{
			Quantity: 1 * unit.Gi,
			Attributes: v2StorageAttributes{
				// types.Attribute{
				// 	Key:   "mount",
				// 	Value: "/usr/local/postgres/data",
				// },
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
			Quantity: 1 * unit.Gi,
			Attributes: v2StorageAttributes{
				// types.Attribute{
				// 	Key:   "mount",
				// 	Value: "/usr/local/mongod/data",
				// },
				types.Attribute{
					Key:   "persistent",
					Value: "true",
				},
			},
		},
	}

	storage.sort()

	// storage without attributes goes on top
	require.Equal(t, byteQuantity(1*unit.Gi), storage[0].Quantity)
	require.Equal(t, 0, len(storage[0].Attributes))

	require.Equal(t, byteQuantity(1*unit.Gi), storage[1].Quantity)
	require.Equal(t, 2, len(storage[1].Attributes))

	attr := types.Attributes(storage[1].Attributes)
	require.Equal(t, "/usr/local/mongod/data", attr[0].Value)

	require.Equal(t, byteQuantity(1*unit.Gi), storage[2].Quantity)
	require.Equal(t, 2, len(storage[2].Attributes))

	attr = types.Attributes(storage[2].Attributes)
	require.Equal(t, "/usr/local/postgres/data", attr[0].Value)
}
