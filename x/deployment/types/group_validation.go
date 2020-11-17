package types

// ValidateDeploymentGroup does validation for provided deployment group
func validateDeploymentGroup(gspec GroupSpec) error {
	if err := validateResourceList(defaultConfig, gspec); err != nil {
		return err
	}
	if err := validateGroupPricing(defaultConfig, gspec); err != nil {
		return err
	}
	return validateOrderBidDuration(defaultConfig, gspec)
}

// ValidateDeploymentGroups does validation for all deployment groups
func ValidateDeploymentGroups(gspecs []GroupSpec) error {
	if len(gspecs) == 0 {
		return ErrInvalidGroups
	}

	for _, group := range gspecs {
		if err := group.ValidateBasic(); err != nil {
			return err
		}
	}

	return nil
}

