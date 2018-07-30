package validation

import (
	"fmt"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
)

func validateResourceListPricing(config config, rlist types.ResourceList) error {
	var (
		mem   int64
		price int64
	)

	for _, resource := range rlist.GetResources() {
		if err := validateResourceGroupPricing(config, resource); err != nil {
			return fmt.Errorf("group %v: %v", rlist.GetName(), err)
		}
		mem += int64(resource.Unit.Memory * uint64(resource.Count))
		price += int64(resource.Price * uint64(resource.Count))
	}

	if price*unit.Gi < mem*config.MinGroupMemPrice {
		return fmt.Errorf("group %v: price too low (%v >= %v fails)",
			rlist.GetName(), price*unit.Gi, mem*config.MinGroupMemPrice)
	}
	return nil
}

func validateResourceGroupPricing(config config, rg types.ResourceGroup) error {
	if rg.Price > uint64(config.MaxUnitPrice) || rg.Price < uint64(config.MinUnitPrice) {
		return fmt.Errorf("error: invalid unit price (%v > %v > %v fails)",
			config.MaxUnitPrice, rg.Price, config.MinUnitPrice)
	}
	return nil
}
