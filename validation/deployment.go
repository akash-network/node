package validation

import (
	"github.com/pkg/errors"

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
		return errors.Wrap(err, "validate deployment group")
	}

	for _, group := range gspecs {
		err := ValidateDeploymentGroup(group)
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateDeploymentGroup does validation for provided deployment group
func ValidateDeploymentGroup(gspec dtypes.GroupSpec) error {
	if err := gspec.ValidateBasic(); err != nil {
		return errors.Wrapf(err, "group validation error: %v", gspec.Name)
	}

	if err := validateResourceList(defaultConfig, gspec); err != nil {
		return err
	}
	if err := validateGroupPricing(defaultConfig, gspec); err != nil {
		return err
	}
	return validateOrderBidDuration(defaultConfig, gspec)
}
