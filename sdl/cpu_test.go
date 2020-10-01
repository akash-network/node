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
	require.Equal(t, "arch", p.Attributes[0].Key)
	require.Equal(t, "amd64", p.Attributes[0].Value)
}
