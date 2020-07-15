package sdl_test

import (
	"testing"

	sdlv1 "github.com/ovrclk/akash/sdl"
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

func Test_v1_Parse_Deployments(t *testing.T) {
	sdl1, err := sdlv1.ReadFile("../x/deployment/testdata/deployment.yaml")
	require.NoError(t, err)
	_, err = sdl1.DeploymentGroups()
	require.NoError(t, err)

	_, err = sdl1.Manifest()
	require.NoError(t, err)

	sha1, err := sdlv1.Version(sdl1)
	require.NoError(t, err)
	assert.Len(t, sha1, 32)

	sha2, err := sdlv1.Version(sdl1)
	require.NoError(t, err)
	assert.Len(t, sha2, 32)

	require.Equal(t, sha1, sha2)

	sdl2, err := sdlv1.ReadFile("../x/deployment/testdata/deployment-v2.yaml")
	require.NoError(t, err)
	sha3, err := sdlv1.Version(sdl2)
	require.NoError(t, err)
	require.NotEqual(t, sha1, sha3)
}

func Test_v1_Parse_simple(t *testing.T) {
	sdl, err := sdlv1.ReadFile("./_testdata/simple.yaml")
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
	sdl, err := sdlv1.ReadFile("./_testdata/profile-svc-name-mismatch.yaml")
	require.NoError(t, err)

	dgroups, err := sdl.DeploymentGroups()
	require.NoError(t, err)
	assert.Len(t, dgroups, 1)

	mani, err := sdl.Manifest()
	require.NoError(t, err)
	assert.Len(t, mani.GetGroups(), 1)
}
