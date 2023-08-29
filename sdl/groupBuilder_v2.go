package sdl

import (
	"sort"

	manifest "github.com/akash-network/akash-api/go/manifest/v2beta2"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

type groupsBuilderV2 struct {
	dgroup        *dtypes.GroupSpec
	mgroup        *manifest.Group
	boundComputes map[string]map[string]int
}

// buildGroups
func (sdl *v2) buildGroups() error {
	endpointsNames := sdl.computeEndpointSequenceNumbers()

	groups := make(map[string]*groupsBuilderV2)

	for _, svcName := range sdl.Deployments.svcNames() {
		depl := sdl.Deployments[svcName]

		for _, placementName := range depl.placementNames() {
			// objects below have been ensured to exist
			svcdepl := depl[placementName]
			compute := sdl.Profiles.Compute[svcdepl.Profile]
			svc := sdl.Services[svcName]
			infra := sdl.Profiles.Placement[placementName]
			price := infra.Pricing[svcdepl.Profile]

			group := groups[placementName]

			if group == nil {
				group = &groupsBuilderV2{
					dgroup: &dtypes.GroupSpec{
						Name: placementName,
					},
					mgroup: &manifest.Group{
						Name: placementName,
					},
					boundComputes: make(map[string]map[string]int),
				}

				group.dgroup.Requirements.Attributes = types.Attributes(infra.Attributes)
				group.dgroup.Requirements.SignedBy = infra.SignedBy

				// keep ordering stable
				sort.Sort(group.dgroup.Requirements.Attributes)

				groups[placementName] = group
			}

			if _, exists := group.boundComputes[placementName]; !exists {
				group.boundComputes[placementName] = make(map[string]int)
			}

			expose, err := sdl.Services[svcName].Expose.toManifestExpose(endpointsNames)
			if err != nil {
				return err
			}

			resources := compute.Resources.toResources()
			resources.Endpoints = expose.GetEndpoints()

			res := compute.Resources.toResources()
			res.Endpoints = expose.GetEndpoints()

			var resID int
			if ln := len(group.dgroup.Resources); ln > 0 {
				resID = ln + 1
			} else {
				resID = 1
			}

			res.ID = uint32(resID)
			resources.ID = res.ID

			group.dgroup.Resources = append(group.dgroup.Resources, dtypes.ResourceUnit{
				Resources: res,
				Price:     price.Value,
				Count:     svcdepl.Count,
			})

			group.boundComputes[placementName][svcdepl.Profile] = len(group.dgroup.Resources) - 1

			msvc := manifest.Service{
				Name:      svcName,
				Image:     svc.Image,
				Args:      svc.Args,
				Env:       svc.Env,
				Resources: resources,
				Count:     svcdepl.Count,
				Command:   svc.Command,
				Expose:    expose,
			}

			if svc.Params != nil {
				params := &manifest.ServiceParams{}

				if len(svc.Params.Storage) > 0 {
					params.Storage = make([]manifest.StorageParams, 0, len(svc.Params.Storage))
					for volName, volParams := range svc.Params.Storage {
						params.Storage = append(params.Storage, manifest.StorageParams{
							Name:     volName,
							Mount:    volParams.Mount,
							ReadOnly: volParams.ReadOnly,
						})
					}
				}

				msvc.Params = params
			}

			group.mgroup.Services = append(group.mgroup.Services, msvc)
		}
	}

	// keep ordering stable
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	sdl.result.dgroups = make(dtypes.GroupSpecs, 0, len(names))
	sdl.result.mgroups = make(manifest.Groups, 0, len(names))

	for _, name := range names {
		mgroup := *groups[name].mgroup
		// stable ordering services by name
		sort.Sort(mgroup.Services)

		sdl.result.dgroups = append(sdl.result.dgroups, groups[name].dgroup)
		sdl.result.mgroups = append(sdl.result.mgroups, mgroup)
	}

	return nil
}
