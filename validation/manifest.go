package validation

import (
	"fmt"
	"github.com/ovrclk/akash/provider/cluster/util"
	"github.com/pkg/errors"
	"regexp"
	"strings"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

// ValidateManifest does validation for manifest
func ValidateManifest(m manifest.Manifest) error {
	return validateManifestGroups(m.GetGroups())
}

type validateManifestGroupsHelper struct {
	hostnames          map[string]int // used as a set
	globalServiceCount uint
}

var ErrInvalidManifest = errors.New("invalid manifest")

func validateManifestGroups(groups []manifest.Group) error {
	helper := validateManifestGroupsHelper{
		hostnames: make(map[string]int),
	}
	names := make(map[string]int) // used as a set
	for _, group := range groups {
		if err := validateManifestGroup(group, &helper); err != nil {
			return err
		}
		if _, exists := names[group.GetName()]; exists {
			return errors.Errorf("duplicate manifest group %q", group.GetName())
		}

		names[group.GetName()] = 0 // Value stored is not used
	}
	if helper.globalServiceCount == 0 {
		return ErrManifestUnreachable
	}
	return nil
}

func validateManifestGroup(group manifest.Group, helper *validateManifestGroupsHelper) error {
	if err := dtypes.ValidateResourceList(group); err != nil {
		return err
	}

	if 0 == len(group.Services) {
		return fmt.Errorf("%w: group %q contains no services", ErrInvalidManifest, group.GetName())
	}

	for _, s := range group.Services {
		if err := validateManifestService(s, helper); err != nil {
			return err
		}
	}
	return nil
}

func validateManifestService(service manifest.Service, helper *validateManifestGroupsHelper) error {
	if len(service.Name) == 0 {
		return ErrServiceNameEmpty
	}

	if len(service.Image) == 0 {
		return errors.Errorf("invalid manifest: service %q has an empty image name", service.Name)
	}

	for _, envVar := range service.Env {
		idx := strings.Index(envVar, "=")
		if idx == 0 {
			return errors.Errorf("invalid manifest: service %q defines an env. var. with an empty name", service.Name)
		}
	}

	if service.Count == 0 {
		return ErrServiceCountIsZero
	}

	for _, serviceExpose := range service.Expose {
		if err := validateServiceExpose(service.Name, serviceExpose, helper); err != nil {
			return err
		}
	}

	return nil
}

func validateServiceExpose(serviceName string, serviceExpose manifest.ServiceExpose, helper *validateManifestGroupsHelper) error {
	if serviceExpose.Port == 0 {
		return ErrServiceExposePortZero
	}

	switch serviceExpose.Proto {
	case manifest.TCP, manifest.UDP:
		break
	default:
		return errors.Errorf("invalid manifest: service %q protocol invalid", serviceName)
	}

	for _, host := range serviceExpose.Hosts {
		if !isValidHostname(host) {
			return ErrServiceExposeInvalidHostname
		}

		_, exists := helper.hostnames[host]
		if exists {
			return errors.Errorf("hostname %q is duplicated, this is not allowed", host)
		}
		helper.hostnames[host] = 0 // Value stored does not matter

		if serviceExpose.Global {
			helper.globalServiceCount++
		}
	}

	return nil
}

var hostnameRegex = regexp.MustCompile("^[[:alnum:],-,\\.]+$")

func isValidHostname(hostname string) bool {
	return hostnameRegex.MatchString(hostname)
}

// ValidateManifestWithGroupSpecs does validation for manifest with group specifications
func ValidateManifestWithGroupSpecs(m *manifest.Manifest, gspecs []*dtypes.GroupSpec) error {
	rlists := make([]types.ResourceGroup, 0, len(gspecs))
	for _, gspec := range gspecs {
		rlists = append(rlists, gspec)
	}
	return validateManifestDeploymentGroups(m.GetGroups(), rlists)
}

// ValidateManifestWithDeployment does basic validation and returns nil
func ValidateManifestWithDeployment(m *manifest.Manifest, dgroups []dtypes.Group) error {
	rgroups := make([]types.ResourceGroup, 0, len(dgroups))
	for _, dgroup := range dgroups {
		rgroups = append(rgroups, dgroup)

	}

	return validateManifestDeploymentGroups(m.GetGroups(), rgroups)
}

func validateManifestDeploymentGroups(mgroups []manifest.Group, dgroups []types.ResourceGroup) error {
	// TODO - audit this to make sure it covers everything
	if len(mgroups) != len(dgroups) {
		return errors.Errorf("invalid manifest: group count mismatch (%v != %v)", len(mgroups), len(dgroups))
	}

	dgroupByName := make(map[string]types.ResourceGroup)

	for _, dgroup := range dgroups {
		dgroupByName[dgroup.GetName()] = dgroup
	}

	for _, mgroup := range mgroups {
		dgroup, dgroupExists := dgroupByName[mgroup.GetName()]

		if !dgroupExists {
			return errors.Errorf("invalid manifest: unknown deployment group ('%v')", mgroup.GetName())
		}

		if err := validateManifestDeploymentGroup(mgroup, dgroup); err != nil {
			return err
		}
	}

	return nil
}

func validateManifestDeploymentGroup(mgroup manifest.Group, dgroup types.ResourceGroup) error {
	mlist := make([]types.Resources, len(mgroup.GetResources()))
	copy(mlist, mgroup.GetResources())

	endpointsCountForDeploymentGroup := 0

	// Iterate over all deployment groups
deploymentGroupLoop:
	for _, drec := range dgroup.GetResources() {
		endpointsCountForDeploymentGroup += len(drec.Resources.Endpoints)
		// Find a matching manifest group
		for idx := range mlist {
			mrec := mlist[idx]

			// Check that this manifest group is not yet exhausted
			if mrec.Count == 0 {
				continue
			}

			// Check that the resources in the deployment group are equal to the manifest group
			if !drec.Resources.Equals(mrec.Resources) {
				continue
			}

			// If the manifest group contains more resources than the deploynent group, then
			// fulfill the deployment group entirely
			if mrec.Count >= drec.Count {
				mrec.Count -= drec.Count
				drec.Count = 0
			} else {
				// Partially fulfill the deployment group since the manifest group contains less
				drec.Count -= mrec.Count
				mrec.Count = 0
			}

			// Update the value stored in the list
			mlist[idx] = mrec

			// If the deployment group is fulfilled then break out and
			// move to the next deployment
			if drec.Count == 0 {
				continue deploymentGroupLoop
			}
		}
		// If this point is reached then the deployment group cannot be fully matched
		// against the given manifest groups
		return errors.Errorf("invalid manifest: unused deployment resources ('%v')", dgroup.GetName())
	}

	// Search for any manifest groups which are not fully satisfied
	for _, mrec := range mlist {
		if mrec.Count > 0 {
			return errors.Errorf("invalid manifest: excess manifest resources ('%v')", mgroup.GetName())
		}
	}

	endpointsCountForManifestGroup := 0
	for _, service := range mgroup.Services {
		for _, serviceExpose := range service.Expose {
			if serviceExpose.Global && !util.ShouldExpose(&serviceExpose) {
				endpointsCountForManifestGroup++
			}
		}
	}

	if endpointsCountForManifestGroup != endpointsCountForDeploymentGroup {
		return errors.Errorf("invalid manifest: mismatch on number of endpoints %d != %d", endpointsCountForManifestGroup, endpointsCountForDeploymentGroup)
	}

	return nil
}
