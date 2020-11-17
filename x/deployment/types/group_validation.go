package types

import "github.com/pkg/errors"

// ValidateDeploymentGroup does validation for provided deployment group
func validateDeploymentGroup(gspec GroupSpec) error {
	if err := ValidateResourceList(gspec); err != nil {
		return err
	}
	if err := validateGroupPricing(gspec); err != nil {
		return err
	}
	return validateOrderBidDuration(gspec)
}

// ValidateDeploymentGroups does validation for all deployment groups
func ValidateDeploymentGroups(gspecs []GroupSpec) error {
	if len(gspecs) == 0 {
		return ErrInvalidGroups
	}

	names := make(map[string]int)
	for _, group := range gspecs {
		if err := group.ValidateBasic(); err != nil {
			return err
		}

		if _, exists := names[group.GetName()]; exists {
			return errors.Errorf("duplicate deployment group name %q", group.GetName())
		}
	}

	return nil
}

