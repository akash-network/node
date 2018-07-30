package manifest

import (
	"fmt"

	"github.com/ovrclk/akash/types"
)

func ValidateWithDeployment(m *types.Manifest, dgroups []*types.DeploymentGroup) error {
	return validateResourceGroups(m.Groups, dgroups)
}

func validateResourceGroups(mgroups []*types.ManifestGroup, dgroups []*types.DeploymentGroup) error {
	if len(mgroups) != len(dgroups) {
		return fmt.Errorf("invalid manifest: group count mismatch (%v != %v)", len(mgroups), len(dgroups))
	}

mainloop:
	for _, mgroup := range mgroups {
		for _, dgroup := range dgroups {
			if mgroup.Name != dgroup.Name {
				continue
			}
			if err := validateResourceGroup(mgroup, dgroup); err != nil {
				return err
			}
			continue mainloop
		}
		return fmt.Errorf("invalid manifest: unknown deployment group ('%v')", mgroup.Name)
	}
	return nil
}

func validateResourceGroup(mgroup types.ResourceList, dgroup types.ResourceList) error {
	mlist := make([]types.ResourceGroup, len(mgroup.GetResources()))
	copy(mlist, mgroup.GetResources())

mainloop:
	for _, drec := range dgroup.GetResources() {

		for idx := range mlist {
			mrec := mlist[idx]

			if mrec.Count == 0 {
				continue
			}

			if !drec.Unit.Equal(mrec.Unit) {
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
		return fmt.Errorf("invalid manifest: unused deployment resources ('%v')", dgroup.GetName())
	}

	for _, mrec := range mlist {
		if mrec.Count > 0 {
			return fmt.Errorf("invalid manifest: excess manifest resources ('%v')", mgroup.GetName())
		}
	}

	return nil
}
