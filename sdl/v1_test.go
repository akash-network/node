package sdl_test

import (
	"math"
	"testing"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_v1_Parse_docs(t *testing.T) {
	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)
	_, err = sdl.DeploymentGroups()
	require.NoError(t, err)

	_, err = sdl.Manifest()
	require.NoError(t, err)
}

func Test_v1_Parse_simple(t *testing.T) {
	sdl, err := sdl.ReadFile("_testdata/simple.yml")
	require.NoError(t, err)

	groups, err := sdl.DeploymentGroups()
	require.NoError(t, err)
	assert.Len(t, groups, 1)

	group := groups[0]
	assert.Len(t, group.GetRequirements(), 1)

	assert.Equal(t, types.ProviderAttribute{
		Name:  "region",
		Value: "us-west",
	}, group.GetRequirements()[0])

	assert.Len(t, group.GetResources(), 1)

	assert.Equal(t, types.ResourceGroup{
		Count: 20,
		Price: 800,
		Unit: types.ResourceUnit{
			CPU:    2000,
			Memory: 128 * uint64(math.Pow(1024, 2)),
			Disk:   5 * uint64(math.Pow(1024, 3)),
		},
	}, group.GetResources()[0])

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	assert.Len(t, mani.GetGroups(), 1)

	assert.Equal(t, &types.ManifestGroup{
		Name: "westcoast",
		Services: []*types.ManifestService{
			{
				Name:  "web",
				Image: "nginx",
				Unit: &types.ResourceUnit{
					CPU:    2000,
					Memory: 128 * uint64(math.Pow(1024, 2)),
					Disk:   5 * uint64(math.Pow(1024, 3)),
				},
				Count: 20,
				Expose: []*types.ManifestServiceExpose{
					{Port: 80, Global: true},
				},
			},
		},
	}, mani.GetGroups()[0])

}
