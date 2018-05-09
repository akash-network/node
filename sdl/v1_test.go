package sdl_test

import (
	"testing"

	"github.com/ovrclk/akash/sdl"
	"github.com/stretchr/testify/require"
)

func Test_v1_Parse(t *testing.T) {
	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	_, err = sdl.DeploymentGroups()
	require.NoError(t, err)

	_, err = sdl.Manifest()
	require.NoError(t, err)
}
