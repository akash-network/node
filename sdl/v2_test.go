package sdl

import (
	"testing"

	"github.com/ovrclk/akash/validation"

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
			Endpoints: []atypes.Endpoint{
				{
					Kind: atypes.Endpoint_SHARED_HTTP,
				},
				{
					Kind: atypes.Endpoint_RANDOM_PORT,
				},
			},
		},
	}, group.GetResources()[0])

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	assert.Len(t, mani.GetGroups(), 1)

	expectedHosts := make([]string, 1)
	expectedHosts[0] = "ahostname.com"
	defaultHTTPOptions, err := (v2HTTPOptions{}).asManifest()
	require.NoError(t, err)
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
					{Port: 80, Global: true, Proto: manifest.TCP, Hosts: expectedHosts,
						HTTPOptions: defaultHTTPOptions},
					{Port: 12345, Global: true, Proto: manifest.UDP,
						HTTPOptions: defaultHTTPOptions},
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

func Test_v2_Parse_DeploymentNameServiceNameMismatch(t *testing.T) {
	sdl, err := ReadFile("./_testdata/deployment-svc-mismatch.yaml")
	require.Error(t, err)
	require.Nil(t, sdl)
	require.Contains(t, err.Error(), "no service profile named")

	sdl, err = ReadFile("./_testdata/simple2.yaml")
	require.NoError(t, err)
	require.NotNil(t, sdl)

	dgroups, err := sdl.DeploymentGroups()
	require.NoError(t, err)
	assert.Len(t, dgroups, 1)

	mani, err := sdl.Manifest()
	require.NoError(t, err)
	assert.Len(t, mani.GetGroups(), 1)

	require.Equal(t, dgroups[0].Name, mani.GetGroups()[0].Name)
	// SDL lists 2 services, but particular deployment specifies only one
	require.Len(t, mani.GetGroups()[0].Services, 1)

	// make sure deployment maps to the right service
	require.Len(t, mani.GetGroups()[0].Services[0].Expose, 2)
	require.Len(t, mani.GetGroups()[0].Services[0].Expose[0].Hosts, 1)
	require.Equal(t, mani.GetGroups()[0].Services[0].Expose[0].Hosts[0], "ahostname.com")
}

func Test_V2_Parse_MultipleServiceTo(t *testing.T) {
	obj, err := ReadFile("./_testdata/multiple_service_to.yaml")
	require.NoError(t, err)
	require.NotNil(t, obj)

	m, err := obj.Manifest()
	require.NoError(t, err)
	require.NotNil(t, m)

	g := m.GetGroups()[0]

	s := g.Services[0]
	require.Equal(t, "hello-world", s.Name)
	require.Len(t, s.Expose, 2)
}

func Test_V2_Parse_MultipleServiceToMultipleDeploy(t *testing.T) {
	obj, err := ReadFile("./_testdata/multiple_service_to_multiple_deploy.yaml")
	require.Error(t, err)
	require.Equal(t, err.Error(), `hello-world.dcloud1: cannot expose to "test-1", no service by that name in this deployment group`)
	require.Nil(t, obj)
}

func TestV2HTTPOptionsAny(t *testing.T) {
	require.False(t, (v2HTTPOptions{}).any())

	require.True(t, (v2HTTPOptions{
		NextCases: []string{nextCase400},
	}).any())

	require.True(t, (v2HTTPOptions{
		NextTimeout: 1,
	}).any())

	require.True(t, (v2HTTPOptions{
		MaxBodySize: 1,
	}).any())

	require.True(t, (v2HTTPOptions{
		ReadTimeout: 1,
	}).any())

	require.True(t, (v2HTTPOptions{
		SendTimeout: 1,
	}).any())
}

func TestV2HTTPOptionsAsManifest(t *testing.T) {
	options := v2HTTPOptions{
		MaxBodySize: 1,
		ReadTimeout: 2,
		SendTimeout: 3,
		NextTries:   4,
		NextTimeout: 5,
		NextCases:   defaultNextCases,
	}

	m, err := options.asManifest()
	require.NoError(t, err)

	require.Equal(t, manifest.ServiceExposeHTTPOptions{
		MaxBodySize: 1,
		ReadTimeout: 2,
		SendTimeout: 3,
		NextTries:   4,
		NextTimeout: 5,
		NextCases:   defaultNextCases,
	}, m)

	options = v2HTTPOptions{
		MaxBodySize: upperLimitBodySize + 1,
	}
	_, err = options.asManifest()
	require.ErrorIs(t, err, errHTTPOptionNotAllowed)

	options = v2HTTPOptions{
		ReadTimeout: upperLimitReadTimeout + 1,
	}
	_, err = options.asManifest()
	require.ErrorIs(t, err, errHTTPOptionNotAllowed)

	options = v2HTTPOptions{
		SendTimeout: upperLimitSendTimeout + 1,
	}
	_, err = options.asManifest()
	require.ErrorIs(t, err, errHTTPOptionNotAllowed)

	options = v2HTTPOptions{
		NextCases: []string{"kittens"},
	}
	_, err = options.asManifest()
	require.ErrorIs(t, err, errUnknownNextCase)

	options = v2HTTPOptions{
		NextCases: []string{nextCaseOff},
	}
	_, err = options.asManifest()
	require.NoError(t, err)

	options = v2HTTPOptions{
		NextCases: []string{nextCaseOff, "kittens"},
	}
	_, err = options.asManifest()
	require.ErrorIs(t, err, errCannotSpecifyOffAndOtherCases)
}

func TestV2HTTPOptionsParse(t *testing.T) {
	data, err := ReadFile("_testdata/simple_httpoptions.yaml")
	require.NoError(t, err)
	require.NotNil(t, data)

	m, err := data.Manifest()
	require.NoError(t, err)
	g := m.GetGroups()[0]

	svc := g.Services[0]
	expose := svc.Expose[0]
	require.Equal(t, manifest.ServiceExposeHTTPOptions{
		MaxBodySize: 1,
		ReadTimeout: 2,
		SendTimeout: 3,
		NextTries:   4,
		NextTimeout: 5,
		NextCases:   []string{"off"},
	}, expose.HTTPOptions)
}
