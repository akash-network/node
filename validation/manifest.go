package validation

import (
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

// ValidateManifest does validation for manifest
func ValidateManifest(m manifest.Manifest) error {
	return validateManifestGroups(m.GetGroups())
}

func validateManifestGroups(groups []manifest.Group) error {
	// rlists := make([]hasResources, 0, len(groups))
	// for _, group := range groups {
	// 	rlists = append(rlists, group)
	// }
	// if err := validateResourceLists(defaultConfig, rlists); err != nil {
	// 	return fmt.Errorf("manifest groups: %v", err)
	// }
	return nil
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
	// TODO: re-enable
	// rlists := make([]types.ResourceList, 0, len(dgroups))
	// for _, dgroup := range dgroups {
	// 	rlists = append(rlists, dgroup)
	// }
	// return validateManifestDeploymentGroups(m, rlists)
	return nil
}

func validateManifestDeploymentGroups(mgroups []manifest.Group, dgroups []types.ResourceGroup) error {

	if len(mgroups) != len(dgroups) {
		return errors.Errorf("invalid manifest: group count mismatch (%v != %v)", len(mgroups), len(dgroups))
	}

mainloop:
	for _, mgroup := range mgroups {
		for _, dgroup := range dgroups {
			if mgroup.GetName() != dgroup.GetName() {
				continue
			}
			if err := validateManifestDeploymentGroup(mgroup, dgroup); err != nil {
				return err
			}
			continue mainloop
		}
		return errors.Errorf("invalid manifest: unknown deployment group ('%v')", mgroup.GetName())
	}
	return nil
}

func validateManifestDeploymentGroup(mgroup types.ResourceGroup, dgroup types.ResourceGroup) error {
	mlist := make([]types.Resources, len(mgroup.GetResources()))
	copy(mlist, mgroup.GetResources())

mainloop:
	for _, drec := range dgroup.GetResources() {

		for idx := range mlist {
			mrec := mlist[idx]

			if mrec.Count == 0 {
				continue
			}

			if !drec.Resources.Equals(mrec.Resources) {
				continue
			}

			if mrec.Count > drec.Count {
				mrec.Count -= drec.Count
				drec.Count = 0
			} else {
				drec.Count -= mrec.Count
				mrec.Count = 0
			}

			mlist[idx] = mrec

			if drec.Count == 0 {
				continue mainloop
			}
		}
		return errors.Errorf("invalid manifest: unused deployment resources ('%v')", dgroup.GetName())
	}

	for _, mrec := range mlist {
		if mrec.Count > 0 {
			return errors.Errorf("invalid manifest: excess manifest resources ('%v')", mgroup.GetName())
		}
	}

	return nil
}
