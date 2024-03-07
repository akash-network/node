package sdl

import (
	"fmt"
	"path"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"

	manifest "github.com/akash-network/akash-api/go/manifest/v2beta2"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

var _ SDL = (*v2_1)(nil)

type v2_1 struct {
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

func (sdl *v2_1) DeploymentGroups() (dtypes.GroupSpecs, error) {
	return sdl.result.dgroups, nil
}

func (sdl *v2_1) Manifest() (manifest.Manifest, error) {
	return manifest.Manifest(sdl.result.mgroups), nil
}

// Version creates the deterministic Deployment Version hash from the SDL.
func (sdl *v2_1) Version() ([]byte, error) {
	return manifest.Manifest(sdl.result.mgroups).Version()
}

func (sdl *v2_1) UnmarshalYAML(node *yaml.Node) error {
	result := v2_1{}

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

func (sdl *v2_1) validate() error {
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

			if svc.Params != nil {
				mounts := make(map[string]string)

				for name, params := range svc.Params.Storage {

					volume, exists := volumes[name]

					if !exists {
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

					if vlname, exists := mounts[params.Mount]; exists {
						if params.Mount == "" {
							return errStorageMultipleRootEphemeral
						}

						return fmt.Errorf(
							"%w: mount %q already in use by volume %q",
							errStorageDupMountPoint,
							params.Mount,
							vlname,
						)
					}

					mounts[params.Mount] = name

					attr := make(map[string]string)
					attr[StorageAttributeMount] = params.Mount
					attr[StorageAttributeReadOnly] = strconv.FormatBool(params.ReadOnly)

					for _, nd := range types.Attributes(volume.Attributes) {
						attr[nd.Key] = nd.Value
					}

					persistent, _ := strconv.ParseBool(attr[StorageAttributePersistent])
					class := attr[StorageAttributeClass]

					if persistent && params.Mount == "" {
						return fmt.Errorf(
							"%w: compute.storage.%s has persistent=true which requires service.%s.params.storage.%s to have mount",
							errSDLInvalid,
							name,
							svcName,
							name,
						)
					}

					if class == StorageClassRAM && params.ReadOnly {
						return fmt.Errorf(
							"%w: services.%s.params.storage.%s has readOnly=true which is not allowed for storage class \"%s\"",
							errSDLInvalid,
							svcName,
							name,
							class,
						)
					}
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

func (sdl *v2_1) computeEndpointSequenceNumbers() map[string]uint32 {
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
