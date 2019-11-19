package validation

import (
	"fmt"

	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

type hasResources interface {
	GetName() string
	GetResources() []dtypes.Resource
}

func ValidateDeploymentGroups(groups []dtypes.GroupSpec) error {
	rlists := make([]hasResources, 0, len(groups))
	for _, group := range groups {
		rlists = append(rlists, group)
	}
	if err := validateDeploymentResourceLists(defaultConfig, rlists); err != nil {
		return fmt.Errorf("deployment groups: %v", err)
	}
	return nil
}

func ValidateDeploymentGroup(group dtypes.Group) error {
	if err := validateResourceList(defaultConfig, group); err != nil {
		return err
	}
	if err := validateResourceListPricing(defaultConfig, group); err != nil {
		return err
	}
	return nil
}

func ValidateGroupSpecs(groups []*dtypes.GroupSpec) error {
	rlists := make([]hasResources, 0, len(groups))
	for _, group := range groups {
		rlists = append(rlists, group)
	}
	if err := validateDeploymentResourceLists(defaultConfig, rlists); err != nil {
		return fmt.Errorf("group specs: %v", err)
	}
	return nil
}

func validateDeploymentResourceLists(config config, rlists []hasResources) error {
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
