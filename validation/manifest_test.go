package validation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

const (
	randCPU1    uint64 = 10
	randCPU2    uint64 = 5
	randMemory  uint64 = 20
	randStorage uint64 = 5
)

var randUnits1 = types.ResourceUnits{
	CPU: &types.CPU{
		Units: types.NewResourceValue(randCPU1),
	},
	Memory: &types.Memory{
		Quantity: types.NewResourceValue(randMemory),
	},
	Storage: &types.Storage{
		Quantity: types.NewResourceValue(randStorage),
	},
}

var randUnits2 = types.ResourceUnits{
	CPU: &types.CPU{
		Units: types.NewResourceValue(randCPU2),
	},
	Memory: &types.Memory{
		Quantity: types.NewResourceValue(randMemory),
	},
	Storage: &types.Storage{
		Quantity: types.NewResourceValue(randStorage),
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
