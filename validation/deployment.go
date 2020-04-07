package validation

import (
	"fmt"

	"github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

// ValidateDeploymentGroups does validation for all deployment groups
func ValidateDeploymentGroups(gspecs []dtypes.GroupSpec) error {
	rlists := make([]types.ResourceGroup, 0, len(gspecs))
	for _, group := range gspecs {
		rlists = append(rlists, group)
	}

	if err := validateResourceLists(defaultConfig, rlists); err != nil {
		return fmt.Errorf("deployment groups: %v", err)
	}

	for _, group := range gspecs {
		if err := validateGroupPricing(defaultConfig, group); err != nil {
			return err
		}
	}
	return nil
}

// ValidateDeploymentGroup does validation for provided deployment group
func ValidateDeploymentGroup(gspec dtypes.GroupSpec) error {
	if err := validateResourceList(defaultConfig, gspec); err != nil {
		return err
	}
	if err := validateGroupPricing(defaultConfig, gspec); err != nil {
		return err
	}
	return nil
}
