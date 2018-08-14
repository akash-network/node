package testutil

import (
	"fmt"
	"testing"

	"github.com/ovrclk/akash/types"
)

func ManifestGroupsForDeploymentGroups(t *testing.T, dgroups []*types.DeploymentGroup) []*types.ManifestGroup {
	mgroups := make([]*types.ManifestGroup, 0, len(dgroups))

	for _, dgroup := range dgroups {
		mgroup := &types.ManifestGroup{Name: dgroup.Name}
		for idx, resource := range dgroup.Resources {
			mgroup.Services = append(mgroup.Services, &types.ManifestService{
				Name:  fmt.Sprintf("svc-%v", idx),
				Unit:  &resource.Unit,
				Count: resource.Count,
			})
		}
		mgroups = append(mgroups, mgroup)
	}
	return mgroups
}
