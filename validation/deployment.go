package validation

import (
	"fmt"

	"github.com/ovrclk/akash/types"
)

func ValidateDeploymentGroups(groups []*types.DeploymentGroup) error {
	rlists := make([]types.ResourceList, 0, len(groups))
	for _, group := range groups {
		rlists = append(rlists, group)
	}
	if err := validateResourceLists(defaultConfig, rlists); err != nil {
		return fmt.Errorf("deployment groups: %v", err)
	}
	return nil
}

func ValidateGroupSpecs(groups []*types.GroupSpec) error {
	rlists := make([]types.ResourceList, 0, len(groups))
	for _, group := range groups {
		rlists = append(rlists, group)
	}
	if err := validateResourceLists(defaultConfig, rlists); err != nil {
		return fmt.Errorf("group specs: %v", err)
	}
	return nil
}
