package sdl_test

import (
	"testing"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	randCPU     uint32 = 100
	randMemory  uint64 = 128 * unit.Mi
	randStorage uint64 = 1 * unit.Gi
)

func Test_v1_Parse_docs(t *testing.T) {
	sdl, err := sdl.ReadFile("../x/deployment/testdata/deployment.yml")
	require.NoError(t, err)
	_, err = sdl.DeploymentGroups()
	require.NoError(t, err)

	_, err = sdl.Manifest()
	require.NoError(t, err)
}

func Test_v1_Parse_simple(t *testing.T) {
	sdl, err := sdl.ReadFile("./_testdata/simple.yml")
	require.NoError(t, err)

	groups, err := sdl.DeploymentGroups()
	require.NoError(t, err)
	assert.Len(t, groups, 1)

	group := groups[0]
	// assert.Len(t, group.GetRequirements(), 1)

	// assert.Equal(t, types.ProviderAttribute{
	// 	Name:  "region",
	// 	Value: "us-west",
	// }, group.GetRequirements()[0])

	assert.Len(t, group.GetResources(), 1)

	assert.Equal(t, types.Resource{
		Count: 2,
		Unit: types.Unit{
			CPU:     randCPU,
			Memory:  randMemory,
			Storage: randStorage,
		},
	}, group.GetResources()[0])

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	assert.Len(t, mani.GetGroups(), 1)

	// assert.Equal(t, &types.ManifestGroup{
	// 	Name: "westcoast",
	// 	Services: []*types.ManifestService{
	// 		{
	// 			Name:  "web",
	// 			Image: "nginx",
	// 			Unit: &types.ResourceUnit{
	// 				CPU:     100,
	// 				Memory:  128 * unit.Mi,
	// 				Storage: 1 * unit.Gi,
	// 			},
	// 			Count: 2,
	// 			Expose: []*types.ManifestServiceExpose{
	// 				{Port: 80, Global: true},
	// 			},
	// 		},
	// 	},
	// }, mani.GetGroups()[0])

}

func Test_v1_Parse_ProfileNameNotServiceName(t *testing.T) {
	sdl, err := sdl.ReadFile("./_testdata/profile-svc-name-mismatch.yml")
	require.NoError(t, err)

	dgroups, err := sdl.DeploymentGroups()
	require.NoError(t, err)
	assert.Len(t, dgroups, 1)

	mani, err := sdl.Manifest()
	require.NoError(t, err)
	assert.Len(t, mani.GetGroups(), 1)
}
