package sdl

import (
	"github.com/ovrclk/akash/validation"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/ovrclk/akash/manifest"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
)

func TestV2Expose(t *testing.T) {
	var stream = `
- port: 80
  as: 80
  accept:
    - hello.localhost
  to:
    - global: true
`

	var p []v2Expose

	err := yaml.Unmarshal([]byte(stream), &p)
	require.NoError(t, err)
}

const (
	randCPU     uint64 = 100
	randMemory  uint64 = 128 * unit.Mi
	randStorage uint64 = 1 * unit.Gi
)

func TestV2Parse_Deployments(t *testing.T) {
	sdl1, err := ReadFile("../x/deployment/testdata/deployment.yaml")
	require.NoError(t, err)
	_, err = sdl1.DeploymentGroups()
	require.NoError(t, err)

	_, err = sdl1.Manifest()
	require.NoError(t, err)

	sha1, err := Version(sdl1)
	require.NoError(t, err)
	assert.Len(t, sha1, 32)

	sha2, err := Version(sdl1)
	require.NoError(t, err)
	assert.Len(t, sha2, 32)

	require.Equal(t, sha1, sha2)

	sdl2, err := ReadFile("../x/deployment/testdata/deployment-v2.yaml")
	require.NoError(t, err)
	sha3, err := Version(sdl2)
	require.NoError(t, err)
	require.NotEqual(t, sha1, sha3)
}

func Test_V2_Cross_Validates(t *testing.T) {
	sdl2, err := ReadFile("../x/deployment/testdata/deployment-v2.yaml")
	require.NoError(t, err)
	dgroups, err := sdl2.DeploymentGroups()
	require.NoError(t, err)
	manifest, err := sdl2.Manifest()
	require.NoError(t, err)

	// This is a single document producing both the manifest & deployment groups
	// These should always agree with each other. If this test fails at least one of the
	// following is ture
	// 1. Cross validation logic is wrong
	// 2. The DeploymentGroups() & Manifest() code do not agree with one another
	err = validation.ValidateManifestWithGroupSpecs(&manifest, dgroups)
	require.NoError(t, err)

	// Repeat the same test with another file
	sdl2, err = ReadFile("./_testdata/simple.yaml")
	require.NoError(t, err)
	dgroups, err = sdl2.DeploymentGroups()
	require.NoError(t, err)
	manifest, err = sdl2.Manifest()
	require.NoError(t, err)

	// This is a single document producing both the manifest & deployment groups
	// These should always agree with each other
	err = validation.ValidateManifestWithGroupSpecs(&manifest, dgroups)
	require.NoError(t, err)

	// Repeat the same test with another file
	sdl2, err = ReadFile("./_testdata/private_service.yaml")
	require.NoError(t, err)
	dgroups, err = sdl2.DeploymentGroups()
	require.NoError(t, err)
	manifest, err = sdl2.Manifest()
	require.NoError(t, err)

	// This is a single document producing both the manifest & deployment groups
	// These should always agree with each other
	err = validation.ValidateManifestWithGroupSpecs(&manifest, dgroups)
	require.NoError(t, err)

}

func Test_v1_Parse_simple(t *testing.T) {
	sdl, err := ReadFile("./_testdata/simple.yaml")
	require.NoError(t, err)

	groups, err := sdl.DeploymentGroups()
	require.NoError(t, err)
	assert.Len(t, groups, 1)

	group := groups[0]
	assert.Len(t, group.GetResources(), 1)

	assert.Equal(t, atypes.Attribute{
		Key:   "region",
		Value: "us-west",
	}, group.Requirements.Attributes[0])

	assert.Len(t, group.GetResources(), 1)

	assert.Equal(t, atypes.Resources{
		Count: 2,
		Resources: atypes.ResourceUnits{
			CPU: &atypes.CPU{
				Units: atypes.NewResourceValue(randCPU),
			},
			Memory: &atypes.Memory{
				Quantity: atypes.NewResourceValue(randMemory),
			},
			Storage: &atypes.Storage{
				Quantity: atypes.NewResourceValue(randStorage),
			},
			Endpoints: make([]atypes.Endpoint, 1),
		},
	}, group.GetResources()[0])

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	assert.Len(t, mani.GetGroups(), 1)

	assert.Equal(t, manifest.Group{
		Name: "westcoast",
		Services: []manifest.Service{
			{
				Name:  "web",
				Image: "nginx",
				Resources: atypes.ResourceUnits{
					CPU: &atypes.CPU{
						Units: atypes.NewResourceValue(100),
					},
					Memory: &atypes.Memory{
						Quantity: atypes.NewResourceValue(128 * unit.Mi),
					},
					Storage: &atypes.Storage{
						Quantity: atypes.NewResourceValue(1 * unit.Gi),
					},
				},
				Count: 2,
				Expose: []manifest.ServiceExpose{
					{Port: 80, Global: true, Proto: manifest.TCP},
					{Port: 12345, Global: true, Proto: manifest.UDP},
				},
			},
		},
	}, mani.GetGroups()[0])
}

func Test_v1_Parse_ProfileNameNotServiceName(t *testing.T) {
	sdl, err := ReadFile("./_testdata/profile-svc-name-mismatch.yaml")
	require.NoError(t, err)

	dgroups, err := sdl.DeploymentGroups()
	require.NoError(t, err)
	assert.Len(t, dgroups, 1)

	mani, err := sdl.Manifest()
	require.NoError(t, err)
	assert.Len(t, mani.GetGroups(), 1)
}
