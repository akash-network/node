package sdl

import (
	"sort"

	"github.com/pkg/errors"

	"github.com/ovrclk/akash/manifest"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

type v2 struct {
	Include     []string                `yaml:",omitempty"`
	Services    map[string]v2Service    `yaml:"services,omitempty"`
	Profiles    v2profiles              `yaml:"profiles,omitempty"`
	Deployments map[string]v2Deployment `yaml:"deployment"`
}

type v2ExposeTo struct {
	Service string `yaml:"service,omitempty"`
	Global  bool   `yaml:"global,omitempty"`
}

type v2Expose struct {
	Port   uint16
	As     uint16
	Proto  string       `yaml:"proto,omitempty"`
	To     []v2ExposeTo `yaml:"to,omitempty"`
	Accept v2Accept     `yaml:"accept"`
}

type v2Dependency struct {
	Service string `yaml:"service"`
}

type v2Service struct {
	Image        string
	Command      []string       `yaml:",omitempty"`
	Args         []string       `yaml:",omitempty"`
	Env          []string       `yaml:",omitempty"`
	Expose       []v2Expose     `yaml:",omitempty"`
	Dependencies []v2Dependency `yaml:",omitempty"`
}

type v2ServiceDeployment struct {
	// Compute profile name
	Profile string

	// Number of instances
	Count uint32
}

// placement-profile -> { compute-profile, count }
type v2Deployment map[string]v2ServiceDeployment

type v2ProfileCompute struct {
	// todo are compute resources mandatory ?
	Resources *v2ComputeResources `yaml:"resources,omitempty"`
}

type v2ProfilePlacement struct {
	Attributes v2PlacementAttributes `yaml:"attributes"`
	Pricing    v2PlacementPricing    `yaml:"pricing"`
}

type v2profiles struct {
	Compute   map[string]v2ProfileCompute   `yaml:"compute"`
	Placement map[string]v2ProfilePlacement `yaml:"placement"`
}

func (sdl *v2) DeploymentGroups() ([]*dtypes.GroupSpec, error) {
	groups := make(map[string]*dtypes.GroupSpec)

	for _, svcName := range v2DeploymentSvcNames(sdl.Deployments) {
		depl := sdl.Deployments[svcName]

		for _, placementName := range v2DeploymentPlacementNames(depl) {
			svcdepl := depl[placementName]

			compute, ok := sdl.Profiles.Compute[svcdepl.Profile]
			if !ok {
				return nil, errors.Errorf("%v.%v: no compute profile named %v", svcName, placementName, svcdepl.Profile)
			}

			infra, ok := sdl.Profiles.Placement[placementName]
			if !ok {
				return nil, errors.Errorf("%v.%v: no placement profile named %v", svcName, placementName, placementName)
			}

			price, ok := infra.Pricing[svcdepl.Profile]
			if !ok {
				return nil, errors.Errorf("%v.%v: no pricing for profile %v", svcName, placementName, svcdepl.Profile)
			}

			group := groups[placementName]

			if group == nil {
				group = &dtypes.GroupSpec{
					Name: placementName,
				}

				for _, v := range infra.Attributes {
					group.Requirements = append(group.Requirements, v)
				}

				// keep ordering stable
				sort.Slice(group.Requirements, func(i, j int) bool {
					return group.Requirements[i].Key < group.Requirements[j].Key
				})

				groups[placementName] = group
			}

			resources := dtypes.Resource{
				Resources: compute.Resources.toResourceUnits(),
				Price:     price.Value,
				Count:     svcdepl.Count,
			}

			group.Resources = append(group.Resources, resources)

			// TODO: Make a parameter to configure the duration of orders being bid on
			group.OrderBidDuration = dtypes.DefaultOrderBiddingDuration
		}
	}

	// keep ordering stable
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]*dtypes.GroupSpec, 0, len(names))
	for _, name := range names {
		result = append(result, groups[name])
	}

	return result, nil
}

func (sdl *v2) Manifest() (manifest.Manifest, error) {
	groups := make(map[string]*manifest.Group)

	for _, svcName := range v2DeploymentSvcNames(sdl.Deployments) {
		depl := sdl.Deployments[svcName]

		for _, placementName := range v2DeploymentPlacementNames(depl) {
			svcdepl := depl[placementName]

			group := groups[placementName]

			if group == nil {
				group = &manifest.Group{
					Name: placementName,
				}
				groups[group.Name] = group
			}

			compute, ok := sdl.Profiles.Compute[svcdepl.Profile]
			if !ok {
				return nil, errors.Errorf("%v.%v: no compute profile named %v", svcName, placementName, svcdepl.Profile)
			}

			svc, ok := sdl.Services[svcName]
			if !ok {
				return nil, errors.Errorf("%v.%v: no service profile named %v", svcName, placementName, svcName)
			}

			msvc := &manifest.Service{
				Name:      svcName,
				Image:     svc.Image,
				Args:      svc.Args,
				Env:       svc.Env,
				Resources: compute.Resources.toResourceUnits(),
				Count:     svcdepl.Count,
			}

			for _, expose := range svc.Expose {
				for _, to := range expose.To {
					msvc.Expose = append(msvc.Expose, manifest.ServiceExpose{
						Service:      to.Service,
						Port:         expose.Port,
						ExternalPort: expose.As,
						Proto:        expose.Proto,
						Global:       to.Global,
						Hosts:        expose.Accept.Items,
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

			group.Services = append(group.Services, *msvc)

		}
	}

	// stable ordering
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]manifest.Group, 0, len(names))
	for _, name := range names {
		result = append(result, *groups[name])
	}

	return result, nil
}

// stable ordering
func v2DeploymentSvcNames(m map[string]v2Deployment) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// stable ordering
func v2DeploymentPlacementNames(m v2Deployment) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
