package sdl

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"

	manifest "github.com/akash-network/akash-api/go/manifest/v2beta2"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/types/v1beta3"

	sdlutil "github.com/akash-network/node/sdl/util"
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
	endpointKindIP        = "ip"
)

var (
	defaultNextCases                 = []string{nextCaseError, nextCaseTimeout}
	errCannotSpecifyOffAndOtherCases = errors.New("if 'off' is specified, no other cases may be specified")
	errUnknownNextCase               = errors.New("next case is unknown")
	errHTTPOptionNotAllowed          = errors.New("http option not allowed")
	errSDLInvalid                    = errors.New("SDL invalid")

	endpointNameValidationRegex = regexp.MustCompile(`^[[:lower:]]+[[:lower:]-_\d]+$`)
)

type v2 struct {
	Include     []string                `yaml:",omitempty"`
	Services    map[string]v2Service    `yaml:"services,omitempty"`
	Profiles    v2profiles              `yaml:"profiles,omitempty"`
	Deployments map[string]v2Deployment `yaml:"deployment"`
	Endpoints   map[string]v2Endpoint   `yaml:"endpoints"`
}

type v2Endpoint struct {
	Kind string `yaml:"kind"`
}

type v2ExposeTo struct {
	Service     string        `yaml:"service,omitempty"`
	Global      bool          `yaml:"global,omitempty"`
	HTTPOptions v2HTTPOptions `yaml:"http_options"`
	IP          string        `yaml:"ip"`
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
	Port        uint32
	As          uint32
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

func (sdl *v2) computeEndpointSequenceNumbers() map[string]uint32 {
	var endpointNames []string

	for _, serviceName := range v2DeploymentSvcNames(sdl.Deployments) {

		for _, expose := range sdl.Services[serviceName].Expose {
			for _, to := range expose.To {
				if to.Global && len(to.IP) == 0 {
					continue
				}

				endpointNames = append(endpointNames, to.IP)
			}
		}
	}

	ipEndpointNames := make(map[string]uint32)
	if len(endpointNames) == 0 {
		return ipEndpointNames
	}

	// Make the assignment stable
	sort.Strings(endpointNames)

	// Start at zero, so the first assigned one is 1
	endpointSeqNumber := uint32(0)
	for _, name := range endpointNames {
		endpointSeqNumber++
		seqNo := endpointSeqNumber
		ipEndpointNames[name] = seqNo
	}

	return ipEndpointNames
}

func (sdl *v2) DeploymentGroups() ([]*dtypes.GroupSpec, error) {
	groups := make(map[string]*dtypes.GroupSpec)

	ipEndpointNames := sdl.computeEndpointSequenceNumbers()
	for _, svcName := range v2DeploymentSvcNames(sdl.Deployments) {
		depl := sdl.Deployments[svcName]

		for _, placementName := range v2DeploymentPlacementNames(depl) {
			svcdepl := depl[placementName]

			// at this moment compute, infra and price have been checked for existence
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
					if !to.Global {
						continue
					}

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
						IP:           to.IP,
					}

					// Check to see if an IP endpoint is also specified
					if v.Global && len(v.IP) != 0 {
						seqNo := ipEndpointNames[v.IP]
						v.EndpointSequenceNumber = seqNo
						endpoints = append(endpoints,
							types.Endpoint{Kind: types.Endpoint_LEASED_IP,
								SequenceNumber: seqNo})
					}

					kind := types.Endpoint_RANDOM_PORT
					if sdlutil.ShouldBeIngress(v) {
						kind = types.Endpoint_SHARED_HTTP
					}

					endpoints = append(endpoints, types.Endpoint{Kind: kind})
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

	ipEndpointNames := sdl.computeEndpointSequenceNumbers()

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

			// at this moment compute and svc have been checked for existence
			compute := sdl.Profiles.Compute[svcdepl.Profile]
			svc := sdl.Services[svcName]

			manifestResources := toManifestResources(compute.Resources)

			var manifestExpose []manifest.ServiceExpose

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

						var seqNo uint32
						if to.Global && len(to.IP) != 0 {
							_, exists := sdl.Endpoints[to.IP]
							if !exists {
								return nil, fmt.Errorf("%w: unknown endpoint %q", errSDLInvalid, to.IP)
							}

							seqNo = ipEndpointNames[to.IP]
							manifestResources.Endpoints = append(manifestResources.Endpoints, types.Endpoint{
								Kind:           types.Endpoint_LEASED_IP,
								SequenceNumber: seqNo,
							})
						}

						manifestExpose = append(manifestExpose, manifest.ServiceExpose{
							Service:                to.Service,
							Port:                   expose.Port,
							ExternalPort:           expose.As,
							Proto:                  proto,
							Global:                 to.Global,
							Hosts:                  expose.Accept.Items,
							HTTPOptions:            httpOptions,
							IP:                     to.IP,
							EndpointSequenceNumber: seqNo,
						})
					}
				} else { // Nothing explicitly set, fill in without any information from "expose.To"
					manifestExpose = append(manifestExpose, manifest.ServiceExpose{
						Service:      "",
						Port:         expose.Port,
						ExternalPort: expose.As,
						Proto:        proto,
						Global:       false,
						Hosts:        expose.Accept.Items,
						HTTPOptions:  httpOptions,
						IP:           "",
					})
				}
			}

			msvc := manifest.Service{
				Name:      svcName,
				Image:     svc.Image,
				Args:      svc.Args,
				Env:       svc.Env,
				Resources: manifestResources,
				Count:     svcdepl.Count,
				Command:   svc.Command,
				Expose:    manifestExpose,
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

			// stable ordering for the Expose portion
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

	result := make([]manifest.Group, 0, len(names))
	for _, name := range names {
		result = append(result, *groups[name])
	}

	return result, nil
}

func (sdl *v2) validate() error {
	for endpointName, endpoint := range sdl.Endpoints {
		if !endpointNameValidationRegex.MatchString(endpointName) {
			return fmt.Errorf("%w: endpoint named %q is not a valid name", errSDLInvalid, endpointName)
		}

		if len(endpoint.Kind) == 0 {
			return fmt.Errorf("%w: endpoint named %q has no kind", errSDLInvalid, endpointName)
		}

		// Validate endpoint kind, there is only one allowed value for now
		if endpoint.Kind != endpointKindIP {
			return fmt.Errorf("%w: endpoint named %q, unknown kind %q", errSDLInvalid, endpointName, endpoint.Kind)
		}
	}

	endpointsUsed := make(map[string]struct{})
	portsUsed := make(map[string]string)
	for _, svcName := range v2DeploymentSvcNames(sdl.Deployments) {
		depl := sdl.Deployments[svcName]

		for _, placementName := range v2DeploymentPlacementNames(depl) {
			svcdepl := depl[placementName]

			compute, ok := sdl.Profiles.Compute[svcdepl.Profile]
			if !ok {
				return fmt.Errorf("%w: %v.%v: no compute profile named %v", errSDLInvalid, svcName, placementName, svcdepl.Profile)
			}

			infra, ok := sdl.Profiles.Placement[placementName]
			if !ok {
				return fmt.Errorf("%w: %v.%v: no placement profile named %v", errSDLInvalid, svcName, placementName, placementName)
			}

			if _, ok := infra.Pricing[svcdepl.Profile]; !ok {
				return fmt.Errorf("%w: %v.%v: no pricing for profile %v", errSDLInvalid, svcName, placementName, svcdepl.Profile)
			}

			svc, ok := sdl.Services[svcName]
			if !ok {
				return fmt.Errorf("%w: %v.%v: no service profile named %v", errSDLInvalid, svcName, placementName, svcName)
			}

			for _, serviceExpose := range svc.Expose {
				for _, to := range serviceExpose.To {
					// Check to see if an IP endpoint is also specified
					if len(to.IP) != 0 {
						if !to.Global {
							return fmt.Errorf("%w: error on %q if an IP is declared the directive must be declared as global", errSDLInvalid, svcName)
						}
						endpoint, endpointExists := sdl.Endpoints[to.IP]
						if !endpointExists {
							return fmt.Errorf("%w: error on service %q no endpoint named %q exists", errSDLInvalid, svcName, to.IP)
						}

						if endpoint.Kind != endpointKindIP {
							return fmt.Errorf("%w: error on service %q endpoint %q has type %q, should be %q", errSDLInvalid, svcName, to.IP, endpoint.Kind, endpointKindIP)
						}

						endpointsUsed[to.IP] = struct{}{}

						// Endpoint exists. Now check for port collisions across a single endpoint, port, & protocol
						portKey := fmt.Sprintf("%s-%d-%s", to.IP, serviceExpose.As, serviceExpose.Proto)
						otherServiceName, inUse := portsUsed[portKey]
						if inUse {
							return fmt.Errorf("%w: IP endpoint %q port: %d protocol: %s specified by service %q already in use by %q", errSDLInvalid, to.IP, serviceExpose.Port, serviceExpose.Proto, svcName, otherServiceName)
						}
						portsUsed[portKey] = svcName
					}

				}
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
						return fmt.Errorf("%w: service \"%s\" references to no-existing compute volume named \"%s\"", errSDLInvalid, svcName, name)
					}

					if !path.IsAbs(params.Mount) {
						return fmt.Errorf("%w: invalid value for \"service.%s.params.%s.mount\" parameter. expected absolute path", errSDLInvalid, svcName, name)
					}

					attr[StorageAttributeMount] = params.Mount
					attr[StorageAttributeReadOnly] = strconv.FormatBool(params.ReadOnly)

					mount := attr[StorageAttributeMount]
					if vlname, exists := mounts[mount]; exists {
						if mount == "" {
							return errStorageMultipleRootEphemeral
						}

						return fmt.Errorf("%w: mount %q already in use by volume %q", errStorageDupMountPoint, mount, vlname)
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
					return fmt.Errorf("%w: compute.storage.%s has persistent=true which requires service.%s.params.storage.%s to have mount", errSDLInvalid, name, svcName, name)
				}
			}
		}
	}

	for endpointName := range sdl.Endpoints {
		_, inUse := endpointsUsed[endpointName]
		if !inUse {
			return fmt.Errorf("%w: endpoint %q declared but never used", errSDLInvalid, endpointName)
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
