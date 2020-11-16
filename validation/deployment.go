package validation

import (
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

// ValidateDeploymentGroups does validation for all deployment groups
func ValidateDeploymentGroups(gspecs []dtypes.GroupSpec) error {

	for _, group := range gspecs {
		if err := group.ValidateBasic(); err != nil {
			return err
		}

	}

	return nil
}

