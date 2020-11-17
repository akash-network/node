package types

import (
	"github.com/pkg/errors"
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types"
)

var (
	ErrNoGroupsPresent = errors.New("validation: no groups present")
	ErrGroupEmptyName  = errors.New("validation: group has empty name")
)

func ValidateResourceList(rlist types.ResourceGroup) error {
	if rlist.GetName() == "" {
		return ErrGroupEmptyName
	}

	units := rlist.GetResources()

	if count := len(units); count > validationConfig.MaxGroupUnits {
		return errors.Errorf("group %v: too many units (%v > %v)", rlist.GetName(), count, validationConfig.MaxGroupUnits)
	}

	limits := newLimits()

	for _, resource := range units {
		gLimits, err := validateResourceGroup(resource)
		if err != nil {
			return fmt.Errorf("group %v: %w", rlist.GetName(), err)
		}

		gLimits.mul(resource.Count)

		limits.add(gLimits)

		// TODO: validate pricing
		// if idx == 0 {
		// 	price = resource.Price
		// } else {
		// 	if resource.Price.Denom != price.Denom {
		// 		return fmt.Errorf("mixed denominations: (%v != %v)", price.Denom, resource.Price.Denom)
		// 	}
		// }
	}

	if limits.cpu.GT(sdk.NewInt(validationConfig.MaxGroupCPU)) || limits.cpu.LTE(sdk.ZeroInt()) {
		return errors.Errorf("group %v: invalid total cpu (%v > %v > %v fails)",
			rlist.GetName(), validationConfig.MaxGroupCPU, limits.cpu, 0)
	}

	if limits.memory.GT(sdk.NewInt(validationConfig.MaxGroupMemory)) || limits.memory.LTE(sdk.ZeroInt()) {
		return errors.Errorf("group %v: invalid total memory (%v > %v > %v fails)",
			rlist.GetName(), validationConfig.MaxGroupMemory, limits.memory, 0)
	}

	if limits.storage.GT(sdk.NewInt(validationConfig.MaxGroupStorage)) || limits.storage.LTE(sdk.ZeroInt()) {
		return errors.Errorf("group %v: invalid total storage (%v > %v > %v fails)",
			rlist.GetName(), validationConfig.MaxGroupStorage, limits.storage, 0)
	}

	return nil
}

func validateResourceGroup(rg types.Resources) (resourceLimits, error) {
	limits, err := validateResourceUnit(rg.Resources)
	if err != nil {
		return resourceLimits{}, err
	}

	if rg.Count > uint32(validationConfig.MaxUnitCount) || rg.Count < uint32(validationConfig.MinUnitCount) {
		return resourceLimits{}, errors.Errorf("error: invalid unit count (%v > %v > %v fails)",
			validationConfig.MaxUnitCount, rg.Count, validationConfig.MinUnitCount)
	}

	// TODO: validate pricing
	// if !rg.Price.IsPositive() {
	// 	return fmt.Errorf("error: invalid unit price (not positive fails)")
	// }

	return limits, nil
}

func validateResourceUnit( units types.ResourceUnits) (resourceLimits, error) {
	limits := newLimits()

	val, err := validateCPU(units.CPU)
	if err != nil {
		return resourceLimits{}, err
	}
	limits.cpu = limits.cpu.Add(val)

	val, err = validateMemory( units.Memory)
	if err != nil {
		return resourceLimits{}, err
	}
	limits.memory = limits.memory.Add(val)

	val, err = validateStorage(units.Storage)
	if err != nil {
		return resourceLimits{}, err
	}
	limits.storage = limits.storage.Add(val)

	return limits, nil
}

func validateCPU( u *types.CPU) (sdk.Int, error) {
	if u == nil {
		return sdk.Int{}, errors.Errorf("error: invalid unit cpu, cannot be nil")
	}
	if (u.Units.Value() > uint64(validationConfig.MaxUnitCPU)) || (u.Units.Value() < uint64(validationConfig.MinUnitCPU)) {
		return sdk.Int{}, errors.Errorf("error: invalid unit cpu (%v > %v > %v fails)",
			validationConfig.MaxUnitCPU, u.Units.Value(), validationConfig.MinUnitCPU)
	}

	return u.Units.Val, nil
}

func validateMemory(u *types.Memory) (sdk.Int, error) {
	if u == nil {
		return sdk.Int{}, errors.Errorf("error: invalid unit memory, cannot be nil")
	}
	if (u.Quantity.Value() > uint64(validationConfig.MaxUnitMemory)) || (u.Quantity.Value() < uint64(validationConfig.MinUnitMemory)) {
		return sdk.Int{}, errors.Errorf("error: invalid unit memory (%v > %v > %v fails)",
			validationConfig.MaxUnitMemory, u.Quantity.Value(), validationConfig.MinUnitMemory)
	}

	return u.Quantity.Val, nil
}

func validateStorage( u *types.Storage) (sdk.Int, error) {
	if u == nil {
		return sdk.Int{}, errors.Errorf("error: invalid unit storage, cannot be nil")
	}
	if (u.Quantity.Value() > uint64(validationConfig.MaxUnitStorage)) || (u.Quantity.Value() < uint64(validationConfig.MinUnitStorage)) {
		return sdk.Int{}, errors.Errorf("error: invalid unit storage (%v > %v > %v fails)",
			validationConfig.MaxUnitStorage, u.Quantity.Value(), validationConfig.MinUnitStorage)
	}

	return u.Quantity.Val, nil
}

type resourceLimits struct {
	cpu     sdk.Int
	memory  sdk.Int
	storage sdk.Int
}

func newLimits() resourceLimits {
	return resourceLimits{
		cpu:     sdk.ZeroInt(),
		memory:  sdk.ZeroInt(),
		storage: sdk.ZeroInt(),
	}
}

func (u *resourceLimits) add(rhs resourceLimits) {
	u.cpu = u.cpu.Add(rhs.cpu)
	u.memory = u.memory.Add(rhs.memory)
	u.storage = u.storage.Add(rhs.storage)
}

func (u *resourceLimits) mul(count uint32) {
	u.cpu = u.cpu.MulRaw(int64(count))
	u.memory = u.memory.MulRaw(int64(count))
	u.storage = u.storage.MulRaw(int64(count))
}
