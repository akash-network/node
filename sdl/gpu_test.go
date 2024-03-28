package sdl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestV2ResourceGPU_EmptyVendor(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.Error(t, err)
}

func TestV2ResourceGPU_Wildcard(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, gpuQuantity(1), p.Units)
	require.Equal(t, 1, len(p.Attributes))
	require.Equal(t, "vendor/nvidia/model/*", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
}

func TestV2ResourceGPU_SingleModel(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, gpuQuantity(1), p.Units)
	require.Equal(t, 1, len(p.Attributes))
	require.Equal(t, "vendor/nvidia/model/a100", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
}

func TestV2ResourceGPU_SingleModelWithRAM(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80Gi
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, gpuQuantity(1), p.Units)
	require.Equal(t, 1, len(p.Attributes))
	require.Equal(t, "vendor/nvidia/model/a100/ram/80Gi", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
}

func TestV2ResourceGPU_InvalidRAMUnit(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80G
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.Error(t, err)
}

func TestV2ResourceGPU_InterfaceInvalid(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        interface: pciex
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.Error(t, err)
}

func TestV2ResourceGPU_RamWithInterface(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80Gi
        interface: pcie
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, gpuQuantity(1), p.Units)
	require.Equal(t, 1, len(p.Attributes))
	require.Equal(t, "vendor/nvidia/model/a100/ram/80Gi/interface/pcie", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
}

func TestV2ResourceGPU_MultipleModels(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80Gi
      - model: a100
        ram: 40Gi
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, gpuQuantity(1), p.Units)
	require.Equal(t, 2, len(p.Attributes))
	require.Equal(t, "vendor/nvidia/model/a100/ram/40Gi", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
	require.Equal(t, "vendor/nvidia/model/a100/ram/80Gi", p.Attributes[1].Key)
	require.Equal(t, "true", p.Attributes[1].Value)
}

func TestV2ResourceGPU_MultipleModels2(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80Gi
      - model: a100
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, gpuQuantity(1), p.Units)
	require.Equal(t, 2, len(p.Attributes))
	require.Equal(t, "vendor/nvidia/model/a100", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
	require.Equal(t, "vendor/nvidia/model/a100/ram/80Gi", p.Attributes[1].Key)
	require.Equal(t, "true", p.Attributes[1].Value)
}

func TestV2ResourceGPU_MultipleModels3(t *testing.T) {
	var stream = `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a6000
      - model: a40
`
	var p v2ResourceGPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, gpuQuantity(1), p.Units)
	require.Equal(t, 2, len(p.Attributes))
	require.Equal(t, "vendor/nvidia/model/a40", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
	require.Equal(t, "vendor/nvidia/model/a6000", p.Attributes[1].Key)
	require.Equal(t, "true", p.Attributes[1].Value)
}
