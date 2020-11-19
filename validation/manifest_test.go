package validation_test

import (
	"bytes"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ovrclk/akash/manifest"
	akashtypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

const (
	randCPU1    uint64 = 10
	randCPU2    uint64 = 5
	randMemory  uint64 = 20
	randStorage uint64 = 5
)

var randUnits1 = akashtypes.ResourceUnits{
	CPU: &akashtypes.CPU{
		Units: akashtypes.NewResourceValue(randCPU1),
	},
	Memory: &akashtypes.Memory{
		Quantity: akashtypes.NewResourceValue(randMemory),
	},
	Storage: &akashtypes.Storage{
		Quantity: akashtypes.NewResourceValue(randStorage),
	},
}

var randUnits2 = akashtypes.ResourceUnits{
	CPU: &akashtypes.CPU{
		Units: akashtypes.NewResourceValue(randCPU2),
	},
	Memory: &akashtypes.Memory{
		Quantity: akashtypes.NewResourceValue(randMemory),
	},
	Storage: &akashtypes.Storage{
		Quantity: akashtypes.NewResourceValue(randStorage),
	},
}

func Test_ValidateManifest(t *testing.T) {
	tests := []struct {
		name    string
		ok      bool
		mgroups []manifest.Group
		dgroups []*dtypes.GroupSpec
	}{
		{
			name: "empty",
			ok:   true,
		},

		{
			name: "single",
			ok:   true,
			mgroups: []manifest.Group{
				{
					Name: "foo",
					Services: []manifest.Service{
						{
							Name:      "svc1",
							Resources: randUnits1,
							Count:     3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Resources: randUnits1,
							Count:     3,
						},
					},
				},
			},
		},

		{
			name: "multi-mgroup",
			ok:   true,
			mgroups: []manifest.Group{
				{
					Name: "foo",
					Services: []manifest.Service{
						{
							Name:      "svc1",
							Resources: randUnits1,
							Count:     1,
						},
						{
							Name:      "svc1",
							Resources: randUnits1,
							Count:     2,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Resources: randUnits1,
							Count:     3,
						},
					},
				},
			},
		},

		{
			name: "multi-dgroup",
			ok:   true,
			mgroups: []manifest.Group{
				{
					Name: "foo",
					Services: []manifest.Service{
						{
							Name:      "svc1",
							Resources: randUnits1,
							Count:     3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Resources: randUnits1,
							Count:     2,
						},
						{
							Resources: randUnits1,
							Count:     1,
						},
					},
				},
			},
		},

		{
			name: "mismatch-name",
			ok:   false,
			mgroups: []manifest.Group{
				{
					Name: "foo-bad",
					Services: []manifest.Service{
						{
							Name:      "svc1",
							Resources: randUnits1,
							Count:     3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Resources: randUnits1,
							Count:     3,
						},
					},
				},
			},
		},

		{
			name: "mismatch-cpu",
			ok:   false,
			mgroups: []manifest.Group{
				{
					Name: "foo",
					Services: []manifest.Service{
						{
							Name:      "svc1",
							Resources: randUnits2,
							Count:     3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Resources: randUnits1,
							Count:     3,
						},
					},
				},
			},
		},

		{
			name: "mismatch-group-count",
			ok:   false,
			mgroups: []manifest.Group{
				{
					Name: "foo",
					Services: []manifest.Service{
						{
							Name:      "svc1",
							Resources: randUnits2,
							Count:     3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{},
		},
	}

	for _, test := range tests {
		m := manifest.Manifest(test.mgroups)
		err := validation.ValidateManifestWithGroupSpecs(&m, test.dgroups)
		if test.ok {
			assert.NoError(t, err, test.name)
		} else {
			assert.Error(t, err, test.name)
		}
	}
}

func TestNilManifestIsInvalid(t *testing.T) {
	err := validation.ValidateManifest(nil)
	require.Error(t, err)
	require.Regexp(t, "^.*manifest is empty.*$", err)
}

const nameOfTestService = "testService"
const nameOfTestGroup = "testGroup"

func simpleResourceUnits() akashtypes.ResourceUnits {
	return akashtypes.ResourceUnits{
		CPU: &akashtypes.CPU{
			Units: akashtypes.ResourceValue{
				Val: sdk.NewInt(10),
			},
			Attributes: nil,
		},
		Memory: &akashtypes.Memory{
			Quantity: akashtypes.ResourceValue{
				Val: sdk.NewInt(1024),
			},
			Attributes: nil,
		},
		Storage: &akashtypes.Storage{
			Quantity: akashtypes.ResourceValue{
				Val: sdk.NewInt(1025),
			},
			Attributes: nil,
		},
		Endpoints: nil,
	}
}

func simpleManifest() manifest.Manifest {
	expose := make([]manifest.ServiceExpose, 1)
	expose[0].Global = true
	expose[0].Port = 80
	expose[0].Proto = manifest.TCP
	expose[0].Hosts = make([]string, 1)
	expose[0].Hosts[0] = "host.test"
	services := make([]manifest.Service, 1)
	services[0] = manifest.Service{
		Name:      nameOfTestService,
		Image:     "test/image:1.0",
		Command:   nil,
		Args:      nil,
		Env:       nil,
		Resources: simpleResourceUnits(),
		Count:     1,
		Expose:    expose,
	}
	m := make(manifest.Manifest, 1)
	m[0] = manifest.Group{
		Name:     nameOfTestGroup,
		Services: services,
	}

	return m
}

func TestSimpleManifestIsValid(t *testing.T) {
	m := simpleManifest()
	err := validation.ValidateManifest(m)
	require.NoError(t, err)
}

func TestManifestWithNoGlobalServicesIsInvalid(t *testing.T) {
	m := simpleManifest()
	m[0].Services[0].Expose[0].Global = false
	err := validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, "^.*zero global services.*$", err)
}

func TestManifestWithDuplicateHostIsInvalid(t *testing.T) {
	m := simpleManifest()
	hosts := make([]string, 2)
	const hostname = "a.test"
	hosts[0] = hostname
	hosts[1] = hostname
	m[0].Services[0].Expose[0].Hosts = hosts
	err := validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, "^.*hostname.+is duplicated.*$", err)
}

func TestManifestWithBadHostIsInvalid(t *testing.T) {
	m := simpleManifest()
	hosts := make([]string, 2)
	hosts[0] = "bob.test" // valid
	hosts[1] = "-bob"     // invalid
	m[0].Services[0].Expose[0].Hosts = hosts
	err := validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, "^.*invalid hostname.*$", err)
}

func TestManifestWithLongHostIsInvalid(t *testing.T) {
	m := simpleManifest()
	hosts := make([]string, 1)
	buf := &bytes.Buffer{}
	for i := 0 ; i != 255; i++ {
		_, err := buf.WriteRune('a')
		require.NoError(t, err)
	}
	_, err := buf.WriteString(".com")
	require.NoError(t, err)

	hosts[0] = buf.String()
	m[0].Services[0].Expose[0].Hosts = hosts
	err = validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, "^.*invalid hostname.*$", err)
}

func TestManifestWithDuplicateGroupIsInvalid(t *testing.T) {
	mDuplicate := make(manifest.Manifest, 2)
	mDuplicate[0] = simpleManifest()[0]
	mDuplicate[1] = simpleManifest()[0]
	mDuplicate[1].Services[0].Expose[0].Hosts[0] = "anotherhost.test"
	err := validation.ValidateManifest(mDuplicate)
	require.Error(t, err)
	require.Regexp(t, "^.*duplicate group.*$", err)
}

func TestManifestWithNoServicesInvalid(t *testing.T) {
	m := simpleManifest()
	m[0].Services = nil
	err := validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, "^.*contains no services.*$", err)
}

func TestManifestWithEmptyServiceNameInvalid(t *testing.T) {
	m := simpleManifest()
	m[0].Services[0].Name = ""
	err := validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, "^.*service name is empty.*$", err)
}

func TestManifestWithEmptyImageNameInvalid(t *testing.T) {
	m := simpleManifest()
	m[0].Services[0].Image = ""
	err := validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, "^.*service.+has empty image name.*$", err)
}

func TestManifestWithEmptyEnvValueIsValid(t *testing.T) {
	m := simpleManifest()
	envVars := make([]string, 2)
	envVars[0] = "FOO=" // sets FOO to empty string
	m[0].Services[0].Env = envVars
	err := validation.ValidateManifest(m)
	require.NoError(t, err)
}

func TestManifestWithEmptyEnvNameIsInvalid(t *testing.T) {
	m := simpleManifest()
	envVars := make([]string, 2)
	envVars[0] = "=FOO" // invalid
	m[0].Services[0].Env = envVars
	err := validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, `^.*var\. with an empty name.*$`, err)
}

func TestManifestServiceUnknownProtocolIsInvalid(t *testing.T) {
	m := simpleManifest()
	m[0].Services[0].Expose[0].Proto = "ICMP"
	err := validation.ValidateManifest(m)
	require.Error(t, err)
	require.Regexp(t, `^.*protocol .+ unknown.*$`, err)
}
