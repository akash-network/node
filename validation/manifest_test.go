package validation_test

import (
	"testing"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/assert"
)

const (
	randCPU1    uint32 = 10
	randCPU2    uint32 = 5
	randMemory  uint64 = 20
	randStorage uint64 = 5
)

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
							Name: "svc1",
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
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
							Name: "svc1",
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 1,
						},
						{
							Name: "svc1",
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 2,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
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
							Name: "svc1",
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 2,
						},
						{
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 1,
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
							Name: "svc1",
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
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
							Name: "svc1",
							Unit: types.Unit{
								CPU:     randCPU2,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*dtypes.GroupSpec{
				{
					Name: "foo",
					Resources: []dtypes.Resource{
						{
							Unit: types.Unit{
								CPU:     randCPU1,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
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
							Name: "svc1",
							Unit: types.Unit{
								CPU:     randCPU2,
								Memory:  randMemory,
								Storage: randStorage,
							},
							Count: 3,
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
