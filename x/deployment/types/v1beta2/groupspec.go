package v1beta2

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "github.com/ovrclk/akash/types/v1beta2"
	atypes "github.com/ovrclk/akash/x/audit/types/v1beta2"
)

// ValidateBasic asserts non-zero values
// TODO: This is causing an import cycle. I think there is some pattern here I'm missing tho..
func (g GroupSpec) ValidateBasic() error {
	return validateDeploymentGroup(g)
}

// GetResources method returns resources list in group
func (g GroupSpec) GetResources() []types.Resources {
	resources := make([]types.Resources, 0, len(g.Resources))
	for _, r := range g.Resources {
		resources = append(resources, types.Resources{
			Resources: r.Resources,
			Count:     r.Count,
		})
	}

	return resources
}

// GetName method returns group name
func (g GroupSpec) GetName() string {
	return g.Name
}

// Price method returns price of group
func (g GroupSpec) Price() sdk.DecCoin {
	var price sdk.DecCoin
	for idx, resource := range g.Resources {
		if idx == 0 {
			price = resource.FullPrice()
			continue
		}
		price = price.Add(resource.FullPrice())
	}
	return price
}

// MatchResourcesRequirements check if resources attributes match provider's capabilities
func (g GroupSpec) MatchResourcesRequirements(pattr types.Attributes) bool {
	for _, rgroup := range g.GetResources() {
		pgroup := pattr.GetCapabilitiesGroup("storage")
		for _, storage := range rgroup.Resources.Storage {
			if len(storage.Attributes) == 0 {
				continue
			}

			if !storage.Attributes.IN(pgroup) {
				return false
			}
		}
	}

	return true
}

// MatchRequirements method compares provided attributes with specific group attributes.
// Argument provider is a bit cumbersome. First element is attributes from x/provider store
// in case tenant does not need signed attributes at all
// rest of elements (if any) are attributes signed by various auditors
func (g GroupSpec) MatchRequirements(provider []atypes.Provider) bool {
	if (len(g.Requirements.SignedBy.AnyOf) != 0) || (len(g.Requirements.SignedBy.AllOf) != 0) {
		// we cannot match if there is no signed attributes
		if len(provider) < 2 {
			return false
		}

		existingRequirements := make(attributesMatching)

		for _, existing := range provider[1:] {
			existingRequirements[existing.Auditor] = existing.Attributes
		}

		if len(g.Requirements.SignedBy.AllOf) != 0 {
			for _, validator := range g.Requirements.SignedBy.AllOf {
				// if at least one signature does not exist or no match on attributes - requirements cannot match
				if existingAttr, exists := existingRequirements[validator]; !exists ||
					!types.AttributesSubsetOf(g.Requirements.Attributes, existingAttr) {
					return false
				}
			}
		}

		if len(g.Requirements.SignedBy.AnyOf) != 0 {
			for _, validator := range g.Requirements.SignedBy.AnyOf {
				if existingAttr, exists := existingRequirements[validator]; exists &&
					types.AttributesSubsetOf(g.Requirements.Attributes, existingAttr) {
					return true
				}
			}

			return false
		}

		return true
	}

	return types.AttributesSubsetOf(g.Requirements.Attributes, provider[0].Attributes)
}
