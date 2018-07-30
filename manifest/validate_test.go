package manifest

import (
	"testing"

	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
)

func Test_ValidateWithDeployment(t *testing.T) {

	tests := []struct {
		name    string
		ok      bool
		mgroups []*types.ManifestGroup
		dgroups []*types.DeploymentGroup
	}{
		{
			name: "empty",
			ok:   true,
		},

		{
			name: "single",
			ok:   true,
			mgroups: []*types.ManifestGroup{
				{
					Name: "foo",
					Services: []*types.ManifestService{
						{
							Name: "svc1",
							Unit: &types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*types.DeploymentGroup{
				{
					Name: "foo",
					Resources: []types.ResourceGroup{
						{
							Unit: types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
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
			mgroups: []*types.ManifestGroup{
				{
					Name: "foo",
					Services: []*types.ManifestService{
						{
							Name: "svc1",
							Unit: &types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
							},
							Count: 1,
						},
						{
							Name: "svc1",
							Unit: &types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
							},
							Count: 2,
						},
					},
				},
			},
			dgroups: []*types.DeploymentGroup{
				{
					Name: "foo",
					Resources: []types.ResourceGroup{
						{
							Unit: types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
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
			mgroups: []*types.ManifestGroup{
				{
					Name: "foo",
					Services: []*types.ManifestService{
						{
							Name: "svc1",
							Unit: &types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*types.DeploymentGroup{
				{
					Name: "foo",
					Resources: []types.ResourceGroup{
						{
							Unit: types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
							},
							Count: 2,
						},
						{
							Unit: types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
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
			mgroups: []*types.ManifestGroup{
				{
					Name: "foo-bad",
					Services: []*types.ManifestService{
						{
							Name: "svc1",
							Unit: &types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*types.DeploymentGroup{
				{
					Name: "foo",
					Resources: []types.ResourceGroup{
						{
							Unit: types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
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
			mgroups: []*types.ManifestGroup{
				{
					Name: "foo",
					Services: []*types.ManifestService{
						{
							Name: "svc1",
							Unit: &types.ResourceUnit{
								CPU:    5,
								Memory: 20,
								Disk:   5,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*types.DeploymentGroup{
				{
					Name: "foo",
					Resources: []types.ResourceGroup{
						{
							Unit: types.ResourceUnit{
								CPU:    10,
								Memory: 20,
								Disk:   5,
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
			mgroups: []*types.ManifestGroup{
				{
					Name: "foo",
					Services: []*types.ManifestService{
						{
							Name: "svc1",
							Unit: &types.ResourceUnit{
								CPU:    5,
								Memory: 20,
								Disk:   5,
							},
							Count: 3,
						},
					},
				},
			},
			dgroups: []*types.DeploymentGroup{},
		},
	}

	for _, test := range tests {
		m := &types.Manifest{Groups: test.mgroups}
		err := ValidateWithDeployment(m, test.dgroups)

		if test.ok {
			assert.NoError(t, err, test.name)
		} else {
			assert.Error(t, err, test.name)
		}
	}

}
