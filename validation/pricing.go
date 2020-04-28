package validation

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

func validateGroupPricing(config config, gspec dtypes.GroupSpec) error {

	var price sdk.Coin

	mem := sdk.NewInt(0)

	for idx, resource := range gspec.Resources {
		if err := validateUnitPricing(config, resource); err != nil {
			return fmt.Errorf("group %v: %v", gspec.GetName(), err)
		}

		if idx == 0 {
			price = resource.FullPrice()
		} else {
			rprice := resource.FullPrice()
			if rprice.Denom != price.Denom {
				return fmt.Errorf("multi-denonimation group: (%v == %v fails)", rprice.Denom, price.Denom)
			}
			price = price.Add(rprice)
		}

		mem = mem.Add(
			sdk.NewIntFromUint64(resource.Unit.Memory).
				Mul(sdk.NewIntFromUint64(uint64(resource.Count))))
	}

	minprice := mem.Mul(sdk.NewInt(config.MinGroupMemPrice)).
		Quo(sdk.NewInt(unit.Gi))

	if price.Amount.LT(minprice) {
		return fmt.Errorf("group %v: price too low (%v >= %v fails)",
			gspec.GetName(), price, minprice)
	}
	return nil
}

func validateUnitPricing(config config, rg dtypes.Resource) error {

	if rg.Price.Amount.GT(sdk.NewIntFromUint64(uint64(config.MaxUnitPrice))) {
		return fmt.Errorf("error: invalid unit price (%v > %v fails)",
			config.MaxUnitPrice, rg.Price)
	}

	if rg.Price.Amount.GT(sdk.NewIntFromUint64(uint64(config.MaxUnitPrice))) {
		return fmt.Errorf("error: invalid unit price (%v < %v fails)",
			config.MinUnitPrice, rg.Price)
	}
	return nil
}
