package sdl

import (
	"fmt"
	"path"
	"sort"
	"strconv"

	"github.com/pkg/errors"

	"github.com/ovrclk/akash/manifest"
	providerUtil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

const (
	nextCaseError         = "error"
	nextCaseTimeout       = "timeout"
	nextCase500           = "500"
	nextCase502           = "502"
	nextCase503           = "503"
	nextCase504           = "504"
	nextCase403           = "403"
	nextCase404           = "404"
	nextCase400           = "429"
	nextCaseOff           = "off"
	defaultMaxBodySize    = uint32(1048576)
	upperLimitBodySize    = uint32(104857600)
	defaultReadTimeout    = uint32(60000)
	upperLimitReadTimeout = defaultReadTimeout
	defaultSendTimeout    = uint32(60000)
	upperLimitSendTimeout = defaultSendTimeout
	defaultNextTries      = uint32(3)
)

var (
	defaultNextCases                 = []string{nextCaseError, nextCaseTimeout}
	errCannotSpecifyOffAndOtherCases = errors.New("if 'off' is specified, no other cases may be specified")
	errUnknownNextCase               = errors.New("next case is unknown")
	errHTTPOptionNotAllowed          = errors.New("http option not allowed")
)

type v2 struct {
	Include     []string                `yaml:",omitempty"`
	Services    map[string]v2Service    `yaml:"services,omitempty"`
	Profiles    v2profiles              `yaml:"profiles,omitempty"`
	Deployments map[string]v2Deployment `yaml:"deployment"`
}

type v2ExposeTo struct {
	Service     string        `yaml:"service,omitempty"`
	Global      bool          `yaml:"global,omitempty"`
	HTTPOptions v2HTTPOptions `yaml:"http_options"`
}

type v2HTTPOptions struct {
	MaxBodySize uint32   `yaml:"max_body_size"`
	ReadTimeout uint32   `yaml:"read_timeout"`
	SendTimeout uint32   `yaml:"send_timeout"`
	NextTries   uint32   `yaml:"next_tries"`
	NextTimeout uint32   `yaml:"next_timeout"`
	NextCases   []string `yaml:"next_cases"`
}

func (ho v2HTTPOptions) asManifest() (manifest.ServiceExposeHTTPOptions, error) {
	maxBodySize := ho.MaxBodySize
	if 0 == maxBodySize {
		maxBodySize = defaultMaxBodySize
	} else if maxBodySize > upperLimitBodySize {
		return manifest.ServiceExposeHTTPOptions{}, fmt.Errorf("%w: body size cannot be greater than %d bytes", errHTTPOptionNotAllowed, upperLimitBodySize)
	}

	readTimeout := ho.ReadTimeout
	if 0 == readTimeout {
		readTimeout = defaultReadTimeout
	} else if readTimeout > upperLimitReadTimeout {
		return manifest.ServiceExposeHTTPOptions{}, fmt.Errorf("%w: read timeout cannot be greater than %d ms", errHTTPOptionNotAllowed, upperLimitReadTimeout)
	}

	sendTimeout := ho.SendTimeout
	if 0 == sendTimeout {
		sendTimeout = defaultSendTimeout
	} else if sendTimeout > upperLimitSendTimeout {
		return manifest.ServiceExposeHTTPOptions{}, fmt.Errorf("%w: send timeout cannot be greater than %d ms", errHTTPOptionNotAllowed, upperLimitSendTimeout)
	}

	nextTries := ho.NextTries
	if 0 == nextTries {
		nextTries = defaultNextTries
	}

	nextCases := ho.NextCases
	if len(nextCases) == 0 {
		nextCases = defaultNextCases
	} else {
		for _, nextCase := range nextCases {
			switch nextCase {
			case nextCaseOff:
				if len(nextCases) != 1 {
					return manifest.ServiceExposeHTTPOptions{}, errCannotSpecifyOffAndOtherCases
				}
			case nextCaseError:
			case nextCaseTimeout:
			case nextCase500:
			case nextCase502:
			case nextCase503:
			case nextCase504:
			case nextCase403:
			case nextCase404:
			case nextCase400:
			default:
				return manifest.ServiceExposeHTTPOptions{}, fmt.Errorf("%w: %q", errUnknownNextCase, nextCase)
			}
		}
	}

	return manifest.ServiceExposeHTTPOptions{
		MaxBodySize: maxBodySize,
		ReadTimeout: readTimeout,
		SendTimeout: sendTimeout,
		NextTries:   nextTries,
		NextTimeout: ho.NextTimeout,
		NextCases:   nextCases,
	}, nil
}

type v2Expose struct {
	Port        uint16
	As          uint16
	Proto       string        `yaml:"proto,omitempty"`
	To          []v2ExposeTo  `yaml:"to,omitempty"`
	Accept      v2Accept      `yaml:"accept"`
	HTTPOptions v2HTTPOptions `yaml:"http_options"`
}

type v2Dependency struct {
	Service string `yaml:"service"`
}

type v2ServiceParams struct {
	Storage map[string]v2ServiceStorageParams `yaml:"storage,omitempty"`
}

type v2Service struct {
	Image        string
	Command      []string         `yaml:",omitempty"`
	Args         []string         `yaml:",omitempty"`
	Env          []string         `yaml:",omitempty"`
	Expose       []v2Expose       `yaml:",omitempty"`
	Dependencies []v2Dependency   `yaml:",omitempty"`
	Params       *v2ServiceParams `yaml:",omitempty"`
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
	SignedBy   types.SignedBy        `yaml:"signedBy"`
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

			// at this moment compute, infra and price have been check for existence
			compute := sdl.Profiles.Compute[svcdepl.Profile]
			infra := sdl.Profiles.Placement[placementName]
			price := infra.Pricing[svcdepl.Profile]

			group := groups[placementName]

			if group == nil {
				group = &dtypes.GroupSpec{
					Name: placementName,
				}

				group.Requirements.Attributes = infra.Attributes
				group.Requirements.SignedBy = infra.SignedBy

				// keep ordering stable
				sort.Slice(group.Requirements.Attributes, func(i, j int) bool {
					return group.Requirements.Attributes[i].Key < group.Requirements.Attributes[j].Key
				})

				groups[placementName] = group
			}

			resources := dtypes.Resource{
				Resources: compute.Resources.toDGroupResourceUnits(),
				Price:     price.Value,
				Count:     svcdepl.Count,
			}

			endpoints := make([]types.Endpoint, 0)
			for _, expose := range sdl.Services[svcName].Expose {
				for _, to := range expose.To {
					if to.Global {
						proto, err := manifest.ParseServiceProtocol(expose.Proto)
						if err != nil {
							return nil, err
						}
						// This value is created just so it can be passed to the utility function
						v := manifest.ServiceExpose{
							Port:         expose.Port,
							ExternalPort: expose.As,
							Proto:        proto,
							Service:      to.Service,
							Global:       to.Global,
							Hosts:        expose.Accept.Items,
						}

						kind := types.Endpoint_RANDOM_PORT
						if providerUtil.ShouldBeIngress(v) {
							kind = types.Endpoint_SHARED_HTTP
						}

						endpoints = append(endpoints, types.Endpoint{Kind: kind})
					}
				}
			}

			resources.Resources.Endpoints = endpoints
			group.Resources = append(group.Resources, resources)
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

			// at this moment compute and svc have been check for existence
			compute := sdl.Profiles.Compute[svcdepl.Profile]
			svc := sdl.Services[svcName]

			msvc := &manifest.Service{
				Name:      svcName,
				Image:     svc.Image,
				Args:      svc.Args,
				Env:       svc.Env,
				Resources: toManifestResources(compute.Resources),
				Count:     svcdepl.Count,
			}

			for _, expose := range svc.Expose {
				proto, err := manifest.ParseServiceProtocol(expose.Proto)
				if err != nil {
					return manifest.Manifest{}, err
				}

				httpOptions, err := expose.HTTPOptions.asManifest()
				if err != nil {
					return manifest.Manifest{}, err
				}

				if len(expose.To) != 0 {
					for _, to := range expose.To {
						msvc.Expose = append(msvc.Expose, manifest.ServiceExpose{
							Service:      to.Service,
							Port:         expose.Port,
							ExternalPort: expose.As,
							Proto:        proto,
							Global:       to.Global,
							Hosts:        expose.Accept.Items,
							HTTPOptions:  httpOptions,
						})
					}
				} else { // Nothing explicitly set, fill in without any information from "expose.To"
					msvc.Expose = append(msvc.Expose, manifest.ServiceExpose{
						Service:      "",
						Port:         expose.Port,
						ExternalPort: expose.As,
						Proto:        proto,
						Global:       false,
						Hosts:        expose.Accept.Items,
						HTTPOptions:  httpOptions,
					})
				}
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

func (sdl *v2) validate() error {
	for _, svcName := range v2DeploymentSvcNames(sdl.Deployments) {
		depl := sdl.Deployments[svcName]

		for _, placementName := range v2DeploymentPlacementNames(depl) {
			svcdepl := depl[placementName]

			compute, ok := sdl.Profiles.Compute[svcdepl.Profile]
			if !ok {
				return errors.Errorf("sdl: %v.%v: no compute profile named %v", svcName, placementName, svcdepl.Profile)
			}

			infra, ok := sdl.Profiles.Placement[placementName]
			if !ok {
				return errors.Errorf("sdl: %v.%v: no placement profile named %v", svcName, placementName, placementName)
			}

			if _, ok := infra.Pricing[svcdepl.Profile]; !ok {
				return errors.Errorf("sdl: %v.%v: no pricing for profile %v", svcName, placementName, svcdepl.Profile)
			}

			svc, ok := sdl.Services[svcName]
			if !ok {
				return errors.Errorf("sdl: %v.%v: no service profile named %v", svcName, placementName, svcName)
			}

			// validate storage's attributes and parameters
			volumes := make(map[string]v2ResourceStorage)
			for _, volume := range compute.Resources.Storage {
				// making deepcopy here as we gonna merge compute attributes and service parameters for validation below
				attr := make(v2StorageAttributes, len(volume.Attributes))

				copy(attr, volume.Attributes)

				volumes[volume.Name] = v2ResourceStorage{
					Name:       volume.Name,
					Quantity:   volume.Quantity,
					Attributes: attr,
				}
			}

			attr := make(map[string]string)
			mounts := make(map[string]string)

			if svc.Params != nil {
				for name, params := range svc.Params.Storage {
					if _, exists := volumes[name]; !exists {
						return errors.Errorf("sdl: service \"%s\" references to no-existing compute volume named \"%s\"", svcName, name)
					}

					if !path.IsAbs(params.Mount) {
						return errors.Errorf("sdl: invalid value for \"service.%s.params.%s.mount\" parameter. expected absolute path", svcName, name)
					}

					attr[StorageAttributeMount] = params.Mount
					attr[StorageAttributeReadOnly] = strconv.FormatBool(params.ReadOnly)

					mount := attr[StorageAttributeMount]
					if vlname, exists := mounts[mount]; exists {
						if mount == "" {
							return errStorageMultipleRootEphemeral
						}

						return errors.Wrap(errStorageDupMountPoint, fmt.Sprintf("sdl: mount \"%s\" already in use by volume \"%s\"", mount, vlname))
					}

					mounts[mount] = name
				}
			}

			for name, volume := range volumes {
				for _, nd := range types.Attributes(volume.Attributes) {
					attr[nd.Key] = nd.Value
				}

				persistent, _ := strconv.ParseBool(attr[StorageAttributePersistent])

				if persistent && attr[StorageAttributeMount] == "" {
					return errors.Errorf("sdl: compute.storage.%s has persistent=true which requires service.%s.params.storage.%s to have mount", name, svcName, name)
				}
			}
		}
	}

	return nil
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
