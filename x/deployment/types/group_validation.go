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

