package sdl

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	manifest "github.com/akash-network/akash-api/go/manifest/v2beta2"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
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
	errCredentialNoHost              = errors.New("Service Credentials missing Host")
	errCredentialNoUsername          = errors.New("Service Credentials missing Username")
	errCredentialNoPassword          = errors.New("Service Credentials missing Password")
)

var endpointNameValidationRegex = regexp.MustCompile(`^[[:lower:]]+[[:lower:]-_\d]+$`)

var _ SDL = (*v2)(nil)

type v2 struct {
	Include     []string              `yaml:",omitempty"`
	Services    map[string]v2Service  `yaml:"services,omitempty"`
	Profiles    v2profiles            `yaml:"profiles,omitempty"`
	Deployments v2Deployments         `yaml:"deployment"`
	Endpoints   map[string]v2Endpoint `yaml:"endpoints"`

	result struct {
		dgroups dtypes.GroupSpecs
		mgroups manifest.Groups
	}
}

type v2Deployments map[string]v2Deployment

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

	if maxBodySize == 0 {
		maxBodySize = defaultMaxBodySize
	} else if maxBodySize > upperLimitBodySize {
		return manifest.ServiceExposeHTTPOptions{}, fmt.Errorf("%w: body size cannot be greater than %d bytes", errHTTPOptionNotAllowed, upperLimitBodySize)
	}

	readTimeout := ho.ReadTimeout
	if readTimeout == 0 {
		readTimeout = defaultReadTimeout
	} else if readTimeout > upperLimitReadTimeout {
		return manifest.ServiceExposeHTTPOptions{}, fmt.Errorf("%w: read timeout cannot be greater than %d ms", errHTTPOptionNotAllowed, upperLimitReadTimeout)
	}

	sendTimeout := ho.SendTimeout
	if sendTimeout == 0 {
		sendTimeout = defaultSendTimeout
	} else if sendTimeout > upperLimitSendTimeout {
		return manifest.ServiceExposeHTTPOptions{}, fmt.Errorf("%w: send timeout cannot be greater than %d ms", errHTTPOptionNotAllowed, upperLimitSendTimeout)
	}

	nextTries := ho.NextTries
	if nextTries == 0 {
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

type v2Exposes []v2Expose

type v2Dependency struct {
	Service string `yaml:"service"`
}

type v2ServiceParams struct {
	Storage map[string]v2ServiceStorageParams `yaml:"storage,omitempty"`
}

type v2Service struct {
	Image        string
	Command      []string              `yaml:",omitempty"`
	Args         []string              `yaml:",omitempty"`
	Env          []string              `yaml:",omitempty"`
	Expose       v2Exposes             `yaml:",omitempty"`
	Dependencies []v2Dependency        `yaml:",omitempty"`
	Params       *v2ServiceParams      `yaml:",omitempty"`
	Credentials  *v2ServiceCredentials `yaml:",omitempty"`
}

type v2ServiceCredentials struct {
	Host     string `yaml:",omitempty"`
	Email    string `yaml:",omitempty"`
	Username string `yaml:",omitempty"`
	Password string `yaml:",omitempty"`
}

func (c v2ServiceCredentials) validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return errCredentialNoHost
	}
	if strings.TrimSpace(c.Username) == "" {
		return errCredentialNoUsername
	}
	if strings.TrimSpace(c.Password) == "" {
		return errCredentialNoPassword
	}
	return nil
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

func (sdl *v2) DeploymentGroups() (dtypes.GroupSpecs, error) {
	return sdl.result.dgroups, nil
}

func (sdl *v2) Manifest() (manifest.Manifest, error) {
	return manifest.Manifest(sdl.result.mgroups), nil
}

// Version creates the deterministic Deployment Version hash from the SDL.
func (sdl *v2) Version() ([]byte, error) {
	return manifest.Manifest(sdl.result.mgroups).Version()
}

func (sdl *v2) UnmarshalYAML(node *yaml.Node) error {
	result := v2{}

loop:
	for i := 0; i < len(node.Content); i += 2 {
		var val interface{}
		switch node.Content[i].Value {
		case "include":
			val = &result.Include
		case "services":
			val = &result.Services
		case "profiles":
			val = &result.Profiles
		case "deployment":
			val = &result.Deployments
		case "endpoints":
			val = &result.Endpoints
		case sdlVersionField:
			// version is already verified
			continue loop
		default:
			return fmt.Errorf("sdl: unexpected field %s", node.Content[i].Value)
		}

		if err := node.Content[i+1].Decode(val); err != nil {
			return err
		}
	}

	if err := result.buildGroups(); err != nil {
		return err
	}

	*sdl = result

	return nil
}

func (sdl *v2) validate() error {
	for endpointName, endpoint := range sdl.Endpoints {
		if !endpointNameValidationRegex.MatchString(endpointName) {
			return fmt.Errorf(
				"%w: endpoint named %q is not a valid name",
				errSDLInvalid,
				endpointName,
			)
		}

		if len(endpoint.Kind) == 0 {
			return fmt.Errorf("%w: endpoint named %q has no kind", errSDLInvalid, endpointName)
		}

		// Validate endpoint kind, there is only one allowed value for now
		if endpoint.Kind != endpointKindIP {
			return fmt.Errorf(
				"%w: endpoint named %q, unknown kind %q",
				errSDLInvalid,
				endpointName,
				endpoint.Kind,
			)
		}
	}

	endpointsUsed := make(map[string]struct{})
	portsUsed := make(map[string]string)
	for _, svcName := range sdl.Deployments.svcNames() {
		depl := sdl.Deployments[svcName]

		for _, placementName := range v2DeploymentPlacementNames(depl) {
			svcdepl := depl[placementName]

			compute, ok := sdl.Profiles.Compute[svcdepl.Profile]
			if !ok {
				return fmt.Errorf(
					"%w: %v.%v: no compute profile named %v",
					errSDLInvalid,
					svcName,
					placementName,
					svcdepl.Profile,
				)
			}

			infra, ok := sdl.Profiles.Placement[placementName]
			if !ok {
				return fmt.Errorf(
					"%w: %v.%v: no placement profile named %v",
					errSDLInvalid,
					svcName,
					placementName,
					placementName,
				)
			}

			if _, ok := infra.Pricing[svcdepl.Profile]; !ok {
				return fmt.Errorf(
					"%w: %v.%v: no pricing for profile %v",
					errSDLInvalid,
					svcName,
					placementName,
					svcdepl.Profile,
				)
			}

			svc, ok := sdl.Services[svcName]
			if !ok {
				return fmt.Errorf(
					"%w: %v.%v: no service profile named %v",
					errSDLInvalid,
					svcName,
					placementName,
					svcName,
				)
			}

			if svc.Credentials != nil {
				if err := svc.Credentials.validate(); err != nil {
					return fmt.Errorf(
						"%w: %v.%v: %v",
						errSDLInvalid,
						svcName,
						placementName,
						err,
					)
				}
			}

			for _, serviceExpose := range svc.Expose {
				for _, to := range serviceExpose.To {
					// Check to see if an IP endpoint is also specified
					if len(to.IP) != 0 {
						if !to.Global {
							return fmt.Errorf(
								"%w: error on %q if an IP is declared the directive must be declared as global",
								errSDLInvalid,
								svcName,
							)
						}
						endpoint, endpointExists := sdl.Endpoints[to.IP]
						if !endpointExists {
							return fmt.Errorf(
								"%w: error on service %q no endpoint named %q exists",
								errSDLInvalid,
								svcName,
								to.IP,
							)
						}

						if endpoint.Kind != endpointKindIP {
							return fmt.Errorf(
								"%w: error on service %q endpoint %q has type %q, should be %q",
								errSDLInvalid,
								svcName,
								to.IP,
								endpoint.Kind,
								endpointKindIP,
							)
						}

						endpointsUsed[to.IP] = struct{}{}

						// Endpoint exists. Now check for port collisions across a single endpoint, port, & protocol
						portKey := fmt.Sprintf(
							"%s-%d-%s",
							to.IP,
							serviceExpose.As,
							serviceExpose.Proto,
						)
						otherServiceName, inUse := portsUsed[portKey]
						if inUse {
							return fmt.Errorf(
								"%w: IP endpoint %q port: %d protocol: %s specified by service %q already in use by %q",
								errSDLInvalid,
								to.IP,
								serviceExpose.Port,
								serviceExpose.Proto,
								svcName,
								otherServiceName,
							)
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
						return fmt.Errorf(
							"%w: service \"%s\" references to no-existing compute volume named \"%s\"",
							errSDLInvalid,
							svcName,
							name,
						)
					}

					if !path.IsAbs(params.Mount) {
						return fmt.Errorf(
							"%w: invalid value for \"service.%s.params.%s.mount\" parameter. expected absolute path",
							errSDLInvalid,
							svcName,
							name,
						)
					}

					attr[StorageAttributeMount] = params.Mount
					attr[StorageAttributeReadOnly] = strconv.FormatBool(params.ReadOnly)

					mount := attr[StorageAttributeMount]
					if vlname, exists := mounts[mount]; exists {
						if mount == "" {
							return errStorageMultipleRootEphemeral
						}

						return fmt.Errorf(
							"%w: mount %q already in use by volume %q",
							errStorageDupMountPoint,
							mount,
							vlname,
						)
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
					return fmt.Errorf(
						"%w: compute.storage.%s has persistent=true which requires service.%s.params.storage.%s to have mount",
						errSDLInvalid,
						name,
						svcName,
						name,
					)
				}
			}
		}
	}

	for endpointName := range sdl.Endpoints {
		_, inUse := endpointsUsed[endpointName]
		if !inUse {
			return fmt.Errorf(
				"%w: endpoint %q declared but never used",
				errSDLInvalid,
				endpointName,
			)
		}
	}

	return nil
}

func (sdl *v2) computeEndpointSequenceNumbers() map[string]uint32 {
	var endpointNames []string
	res := make(map[string]uint32)

	for _, serviceName := range sdl.Deployments.svcNames() {
		for _, expose := range sdl.Services[serviceName].Expose {
			for _, to := range expose.To {
				if to.Global && len(to.IP) == 0 {
					continue
				}

				endpointNames = append(endpointNames, to.IP)
			}
		}
	}

	if len(endpointNames) == 0 {
		return res
	}

	// Make the assignment stable
	sort.Strings(endpointNames)

	// Start at zero, so the first assigned one is 1
	endpointSeqNumber := uint32(0)
	for _, name := range endpointNames {
		endpointSeqNumber++
		seqNo := endpointSeqNumber
		res[name] = seqNo
	}

	return res
}

func (sdl v2Deployments) svcNames() []string {
	names := make([]string, 0, len(sdl))
	for name := range sdl {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// placementNames stable ordered placement names
func (sdl v2Deployment) placementNames() []string {
	names := make([]string, 0, len(sdl))
	for name := range sdl {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

func v2DeploymentPlacementNames(m v2Deployment) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
