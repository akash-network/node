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

func validateManifestGroups(groups []manifest.Group) error {
	names := make(map[string]int) // used as a set
	 for _, group := range groups {
		if err := validateManifestGroup(group); err != nil {
			return err
		}
		if _, exists := names[group.GetName()]; exists {
			return errors.Errorf("duplicate manifest group %q", group.GetName())
		}

		names[group.GetName()] = 0 // Value stored is not used
	 }

	return nil
}

func validateManifestGroup(group manifest.Group) error {
	if err := dtypes.ValidateResourceList(group); err != nil {
		return fmt.Errorf("manifest groups: %v", err)
	}

	if 0 == len(group.Services) {
		return ErrGroupContainsNoServices
	}

	for _, s := range group.Services {
		if err := validateManifestService(s); err != nil {
			return err
		}
	}
	return nil
}

func validateManifestService(service manifest.Service) error {
	if len(service.Name) == 0 {
		return ErrServiceNameEmpty
	}

	if len(service.Image) == 0 {
		return ErrServiceImageEmpty
	}

	for _, envVar := range service.Env {
		idx := strings.Index(envVar, "=")
		if idx == 0 {
			return ErrServiceEnvVarEmptyName
		}
	}

	if service.Count == 0 {
		return ErrServiceCountIsZero
	}

	for _, serviceExpose := range service.Expose {
		if err := validateServiceExpose(serviceExpose); err != nil {
			return err
		}
	}

	return nil
}

func validateServiceExpose(serviceExpose manifest.ServiceExpose) error {
	if serviceExpose.Port == 0 {
		return ErrServiceExposePortZero
	}

	for _, host := range serviceExpose.Hosts {
		if !isValidHostname(host) {
			return ErrServiceExposeInvalidHostname
		}
	}

	// TODO - validate that the hostnames are unique across the entire manifest
	return nil
}


var hostnameRegex = regexp.MustCompile("^[[:alnum:],-,\\.]+$")

func isValidHostname(hostname string) bool{
	return hostnameRegex.MatchString(hostname)
}

// TODO - I think we can eliminate this ?!
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
			//return errors.Errorf("invalid manifest: unknown deployment group ('%v')", mgroup.GetName())
			return ErrManifestGroupDoesNotExistInDeployment
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
		for _, serviceExpose := range service.Expose{
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
