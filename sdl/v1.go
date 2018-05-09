package sdl

import (
	"fmt"
	"sort"

	"github.com/ovrclk/akash/types"
)

type v1 struct {
	Version  string   `yaml:",omitempty"`
	Include  []string `yaml:",omitempty"`
	Services map[string]v1Service
	Profiles v1Profiles

	// service-name -> { placement-profile -> { compute-profile, count } }
	Deployments map[string]v1Deployment `yaml:"deployment"`
}

type v1Service struct {
	Image        string
	Expose       []v1Expose     `yaml:",omitempty"`
	Dependencies []v1Dependency `yaml:",omitempty"`
}

type v1Expose struct {
	Port  uint32
	Proto string       `yaml:",omitempty"`
	To    []v1ExposeTo `yaml:",omitempty"`
}

type v1ExposeTo struct {
	Service string `yaml:",omitempty"`
	Global  bool   `yaml:",omitempty"`
}

type v1Dependency struct {
	Service string
}

type v1Profiles struct {
	Compute   map[string]v1ComputeProfile
	Placement map[string]v1PlacementProfile
}

type v1ComputeProfile struct {
	CPU    uint32 `yaml:"cpu"`
	Memory uint32
	Disk   uint64
}

type v1PlacementProfile struct {
	Attributes map[string]string
	Pricing    map[string]uint32
}

// placement-profile -> { compute-profile, count }
type v1Deployment map[string]v1ServiceDeployment

type v1ServiceDeployment struct {
	// Compute profile name
	Profile string

	// Number of instances
	Count uint32
}

func (sdl *v1) Validate() error {
	return nil
}

func (sdl *v1) DeploymentGroups() ([]*types.GroupSpec, error) {
	groups := make(map[string]*types.GroupSpec)

	for svcName, depl := range sdl.Deployments {

		for placementName, svcdepl := range depl {

			compute, ok := sdl.Profiles.Compute[svcdepl.Profile]
			if !ok {
				return nil, fmt.Errorf("%v.%v: no compute profile named %v", svcName, placementName, svcdepl.Profile)
			}

			infra, ok := sdl.Profiles.Placement[placementName]
			if !ok {
				return nil, fmt.Errorf("%v.%v: no placement profile named %v", svcName, placementName, placementName)
			}

			price, ok := infra.Pricing[svcName]
			if !ok {
				return nil, fmt.Errorf("%v.%v: no pricing for service %v", svcName, placementName, svcName)
			}

			group := groups[placementName]

			if group == nil {
				group = &types.GroupSpec{}

				for k, v := range infra.Attributes {
					group.Requirements = append(group.Requirements, types.ProviderAttribute{
						Name:  k,
						Value: v,
					})
				}

				// keep ordering stable
				sort.Slice(group.Requirements, func(i, j int) bool {
					return group.Requirements[i].Name < group.Requirements[j].Name
				})

				groups[placementName] = group
			}

			group.Resources = append(group.Resources, types.ResourceGroup{
				Unit: types.ResourceUnit{
					Cpu:    compute.CPU,
					Memory: compute.Memory,
					Disk:   compute.Disk,
				},
				Price: price,
				Count: svcdepl.Count,
			})

		}
	}

	// keep ordering stable
	names := make([]string, 0, len(groups))
	for name, _ := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]*types.GroupSpec, 0, len(names))
	for _, name := range names {
		result = append(result, groups[name])
	}

	return result, nil
}

func (sdl *v1) Manifest() (*types.Manifest, error) {

	groups := make(map[string]*types.ManifestGroup)

	for svcName, depl := range sdl.Deployments {

		for placementName, svcdepl := range depl {

			group := groups[placementName]

			if group == nil {
				group = &types.ManifestGroup{
					Name: placementName,
				}
				groups[group.Name] = group
			}

			compute, ok := sdl.Profiles.Compute[svcdepl.Profile]
			if !ok {
				return nil, fmt.Errorf("%v.%v: no compute profile named %v", svcName, placementName, svcdepl.Profile)
			}

			svc, ok := sdl.Services[svcName]
			if !ok {
				return nil, fmt.Errorf("%v.%v: no service profile named %v", svcName, placementName, svcName)
			}

			msvc := &types.ManifestService{
				Name:  svcName,
				Image: svc.Image,
				Unit: types.ResourceUnit{
					Cpu:    compute.CPU,
					Memory: compute.Memory,
					Disk:   compute.Disk,
				},
				Count: svcdepl.Count,
			}

			for _, expose := range svc.Expose {
				for _, to := range expose.To {
					msvc.Expose = append(msvc.Expose, &types.ManifestServiceExpose{
						Service: to.Service,
						Port:    expose.Port,
						Proto:   expose.Proto,
						Global:  to.Global,
					})
				}
			}

			// stable ordering
			sort.Slice(msvc.Expose, func(i, j int) bool {
				a, b := msvc.Expose[i], msvc.Expose[j]

				if a.Service != b.Service {
					return a.Service < b.Service
				}

				if a.Port != b.Port {
					return a.Port < b.Port
				}

				if a.Proto != b.Proto {
					return a.Proto < b.Proto
				}

				if a.Global != b.Global {
					return a.Global
				}

				return false
			})

			group.Services = append(group.Services, msvc)

		}
	}

	// stable ordering
	names := make([]string, 0, len(groups))
	for name, _ := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]*types.ManifestGroup, 0, len(names))
	for _, name := range names {
		result = append(result, groups[name])
	}

	return &types.Manifest{Groups: result}, nil
}
