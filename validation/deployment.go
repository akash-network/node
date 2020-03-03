package validation

import (
	"fmt"

	"github.com/ovrclk/akash/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

// ValidateDeploymentGroups does validation for all deployment groups
func ValidateDeploymentGroups(groups []dtypes.GroupSpec) error {
	rlists := make([]types.ResourceGroup, 0, len(groups))
	for _, group := range groups {
		rlists = append(rlists, group)
	}
	if err := validateDeploymentResourceLists(defaultConfig, rlists); err != nil {
		return fmt.Errorf("deployment groups: %v", err)
	}
	return nil
}

// ValidateDeploymentGroup does validation for provided deployment group
func ValidateDeploymentGroup(group dtypes.Group) error {
	if err := validateResourceList(defaultConfig, group); err != nil {
		return err
	}
	if err := validateResourceListPricing(defaultConfig, group); err != nil {
		return err
	}
	return nil
}

// ValidateGroupSpecs does validation for provided deployment group specifications
func ValidateGroupSpecs(groups []*dtypes.GroupSpec) error {
	rlists := make([]types.ResourceGroup, 0, len(groups))
	for _, group := range groups {
		rlists = append(rlists, group)
	}
	if err := validateDeploymentResourceLists(defaultConfig, rlists); err != nil {
		return fmt.Errorf("group specs: %v", err)
	}
	return nil
}

// validateDeploymentResourceLists does validation for deployment resources list
func validateDeploymentResourceLists(config config, rlists []types.ResourceGroup) error {
	if err := validateResourceLists(defaultConfig, rlists); err != nil {
		return err
	}
	for _, rlist := range rlists {
		if err := validateResourceListPricing(config, rlist); err != nil {
			return err
		}
	}
	return nil
}
