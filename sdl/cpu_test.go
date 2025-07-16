package sdl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestV2ResourceCPU_Valid(t *testing.T) {
	var stream = `
units: 0.1
attributes:
  arch: amd64
`
	var p v2ResourceCPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, cpuQuantity(100), p.Units)
	require.Equal(t, 1, len(p.Attributes))
	require.Equal(t, "capabilities/cpu/arch/amd64", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
}

func TestV2ResourceCPU_ARM64(t *testing.T) {
	var stream = `
units: 0.5
attributes:
  arch: arm64
`
	var p v2ResourceCPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, cpuQuantity(500), p.Units)
	require.Equal(t, 1, len(p.Attributes))
	require.Equal(t, "capabilities/cpu/arch/arm64", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
}

func TestV2ResourceCPU_ArchSlice(t *testing.T) {
	var stream = `
units: 1
attributes:
  arch:
    - amd64
    - arm64
`
	var p v2ResourceCPU

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
	require.Equal(t, cpuQuantity(1000), p.Units)
	require.Equal(t, 2, len(p.Attributes))
	require.Equal(t, "capabilities/cpu/arch/amd64", p.Attributes[0].Key)
	require.Equal(t, "true", p.Attributes[0].Value)
	require.Equal(t, "capabilities/cpu/arch/arm64", p.Attributes[1].Key)
	require.Equal(t, "true", p.Attributes[1].Value)
}
