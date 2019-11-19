package sdl

import (
	"fmt"
	"sort"

	"github.com/blang/semver"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/tendermint/tendermint/libs/common"
)

var (
	allowedVersion = semver.MustParse("1.5.0")
)

type v1 struct {
	Version  string
	Include  []string `yaml:",omitempty"`
	Services map[string]v1Service
	Profiles v1Profiles

	// service-name -> { placement-profile -> { compute-profile, count } }
	Deployments map[string]v1Deployment `yaml:"deployment"`
}

type v1Service struct {
	Image        string
	Args         []string       `yaml:",omitempty"`
	Env          []string       `yaml:",omitempty"`
	Expose       []v1Expose     `yaml:",omitempty"`
	Dependencies []v1Dependency `yaml:",omitempty"`
}

type v1Expose struct {
	Port   uint32
	As     uint32
	Proto  string       `yaml:",omitempty"`
	To     []v1ExposeTo `yaml:",omitempty"`
	Accept v1Accept
}

type v1Accept struct {
	Items []string `yaml:",omitempty"`
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
	CPU     cpuQuantity `yaml:"cpu"`
	Memory  byteQuantity
	Storage byteQuantity
}

type v1PlacementProfile struct {
	Attributes map[string]string
	Pricing    map[string]v1PricingProfile
}

// TODO: make coin parsing "just work".  wtf.
type v1PricingProfile struct {
	Denom  string
	Amount string
}

func (pp v1PricingProfile) ToCoin() sdk.Coin {
	amt, _ := sdk.NewIntFromString(pp.Amount)
	coin := sdk.NewCoin(pp.Denom, amt)
	return coin
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

	if sdl.Version == "" {
		return fmt.Errorf("invalid version: '%v' required", allowedVersion)
	}

	vsn, err := semver.ParseTolerant(sdl.Version)
	if err != nil {
		return err
	}

	if !allowedVersion.EQ(vsn) {
		return fmt.Errorf("invalid version: '%v' required", allowedVersion)
	}

	return nil
}

func (sdl *v1) DeploymentGroups() ([]*dtypes.GroupSpec, error) {
	groups := make(map[string]*dtypes.GroupSpec)

	for _, svcName := range v1DeploymentSvcNames(sdl.Deployments) {
		depl := sdl.Deployments[svcName]

		for _, placementName := range v1DeploymentPlacementNames(depl) {
			svcdepl := depl[placementName]

			compute, ok := sdl.Profiles.Compute[svcdepl.Profile]
			if !ok {
				return nil, fmt.Errorf("%v.%v: no compute profile named %v", svcName, placementName, svcdepl.Profile)
			}

			infra, ok := sdl.Profiles.Placement[placementName]
			if !ok {
				return nil, fmt.Errorf("%v.%v: no placement profile named %v", svcName, placementName, placementName)
			}

			price, ok := infra.Pricing[svcdepl.Profile]
			if !ok {
				return nil, fmt.Errorf("%v.%v: no pricing for profile %v", svcName, placementName, svcdepl.Profile)
			}

			group := groups[placementName]

			if group == nil {
				group = &dtypes.GroupSpec{
					Name: placementName,
				}

				for k, v := range infra.Attributes {
					group.Requirements = append(group.Requirements, common.KVPair{
						Key:   []byte(k),
						Value: []byte(v),
					})
				}

				// keep ordering stable
				sort.Slice(group.Requirements, func(i, j int) bool {
					return string(group.Requirements[i].Key) < string(group.Requirements[j].Key)
				})

				groups[placementName] = group
			}

			group.Resources = append(group.Resources, dtypes.Resource{
				Unit: dtypes.Unit{
					CPU:     uint32(compute.CPU),
					Memory:  uint64(compute.Memory),
					Storage: uint64(compute.Storage),
				},
				Price: price.ToCoin(),
				Count: svcdepl.Count,
			})

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

func (sdl *v1) Manifest() (*types.Manifest, error) {

	groups := make(map[string]*types.ManifestGroup)

	for _, svcName := range v1DeploymentSvcNames(sdl.Deployments) {
		depl := sdl.Deployments[svcName]

		for _, placementName := range v1DeploymentPlacementNames(depl) {
			svcdepl := depl[placementName]

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
				Args:  svc.Args,
				Env:   svc.Env,
				Unit: &types.ResourceUnit{
					CPU:    uint32(compute.CPU),
					Memory: uint64(compute.Memory),
					Disk:   uint64(compute.Storage),
				},
				Count: svcdepl.Count,
			}

			for _, expose := range svc.Expose {
				for _, to := range expose.To {
					msvc.Expose = append(msvc.Expose, &types.ManifestServiceExpose{
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

			group.Services = append(group.Services, msvc)

		}
	}

	// stable ordering
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]*types.ManifestGroup, 0, len(names))
	for _, name := range names {
		result = append(result, groups[name])
	}

	return &types.Manifest{Groups: result}, nil
}

// stable ordering
func v1DeploymentSvcNames(m map[string]v1Deployment) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// stable ordering
func v1DeploymentPlacementNames(m v1Deployment) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
