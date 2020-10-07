package validation

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

func validateGroupPricing(config ValConfig, gspec dtypes.GroupSpec) error {
	var price sdk.Coin

	mem := sdk.NewInt(0)

	for idx, resource := range gspec.Resources {
		if err := validateUnitPricing(config, resource); err != nil {
			return fmt.Errorf("group %v: %w", gspec.GetName(), err)
		}

		if idx == 0 {
			price = resource.FullPrice()
		} else {
			rprice := resource.FullPrice()
			if rprice.Denom != price.Denom {
				return errors.Errorf("multi-denonimation group: (%v == %v fails)", rprice.Denom, price.Denom)
			}
			price = price.Add(rprice)
		}

		memCount := sdk.NewInt(0)
		if u := resource.Resources.Memory; u != nil {
			memCount.Add(sdk.NewIntFromUint64(u.Quantity.Value()))
		}

		mem = mem.Add(memCount.Mul(sdk.NewIntFromUint64(uint64(resource.Count))))
	}

	minprice := mem.Mul(sdk.NewInt(config.MinGroupMemPrice)).Quo(sdk.NewInt(unit.Gi))

	if price.Amount.LT(minprice) {
		return errors.Errorf("group %v: price too low (%v >= %v fails)", gspec.GetName(), price, minprice)
	}
	return nil
}

func validateUnitPricing(config ValConfig, rg dtypes.Resource) error {
	if !rg.Price.IsValid() {
		return errors.Errorf("error: invalid price object")
	}

	if rg.Price.Amount.GT(sdk.NewIntFromUint64(uint64(config.MaxUnitPrice))) {
		return errors.Errorf("error: invalid unit price (%v > %v fails)", config.MaxUnitPrice, rg.Price)
	}

	if rg.Price.Amount.GT(sdk.NewIntFromUint64(uint64(config.MaxUnitPrice))) {
		return errors.Errorf("error: invalid unit price (%v < %v fails)", config.MinUnitPrice, rg.Price)
	}

	return nil
}

func validateOrderBidDuration(_ ValConfig, rg dtypes.GroupSpec) error {
	if !(rg.OrderBidDuration > 0) {
		return errors.Errorf("error: order bid duration must be greater than zero")
	}
	if rg.OrderBidDuration > dtypes.MaxBiddingDuration {
		return errors.Errorf("error: order bid duration must not be greater than %v", dtypes.MaxBiddingDuration)
	}
	return nil
}
