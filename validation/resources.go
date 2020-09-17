package validation

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/types"
)

var (
	ErrNoGroupsPresent = errors.New("validation: no groups present")
	ErrGroupEmptyName  = errors.New("validation: group has empty name")
)

// ValidateResourceList does basic validation for resources list
func ValidateResourceList(rlist types.ResourceGroup) error {
	return validateResourceList(defaultConfig, rlist)
}

func validateResourceLists(config ValConfig, rlists []types.ResourceGroup) error {
	if len(rlists) == 0 {
		return ErrNoGroupsPresent
	}

	if count := len(rlists); count > config.MaxGroupCount {
		return errors.Errorf("error: too many groups (%v > %v)", count, config.MaxGroupCount)
	}

	names := make(map[string]bool)

	for _, rlist := range rlists {

		if ok := names[rlist.GetName()]; ok {
			return errors.Errorf("error: duplicate name (%v)", rlist.GetName())
		}
		names[rlist.GetName()] = true

		if err := validateResourceList(config, rlist); err != nil {
			return err
		}
	}
	return nil
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

func validateResourceList(config ValConfig, rlist types.ResourceGroup) error {
	if rlist.GetName() == "" {
		return ErrGroupEmptyName
	}

	units := rlist.GetResources()

	if count := len(units); count > config.MaxGroupUnits {
		return errors.Errorf("group %v: too many units (%v > %v)", rlist.GetName(), count, config.MaxGroupUnits)
	}

	limits := newLimits()

	for _, resource := range units {
		gLimits, err := validateResourceGroup(config, resource)
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

	if limits.cpu.GT(sdk.NewInt(config.MaxGroupCPU)) || limits.cpu.LTE(sdk.ZeroInt()) {
		return errors.Errorf("group %v: invalid total cpu (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupCPU, limits.cpu, 0)
	}

	if limits.memory.GT(sdk.NewInt(config.MaxGroupMemory)) || limits.memory.LTE(sdk.ZeroInt()) {
		return errors.Errorf("group %v: invalid total memory (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupMemory, limits.memory, 0)
	}

	if limits.storage.GT(sdk.NewInt(config.MaxGroupStorage)) || limits.storage.LTE(sdk.ZeroInt()) {
		return errors.Errorf("group %v: invalid total storage (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupStorage, limits.storage, 0)
	}

	return nil
}

func validateResourceGroup(config ValConfig, rg types.Resources) (resourceLimits, error) {
	limits, err := validateResourceUnit(config, rg.Resources)
	if err != nil {
		return resourceLimits{}, err
	}

	if rg.Count > uint32(config.MaxUnitCount) || rg.Count < uint32(config.MinUnitCount) {
		return resourceLimits{}, errors.Errorf("error: invalid unit count (%v > %v > %v fails)",
			config.MaxUnitCount, rg.Count, config.MinUnitCount)
	}

	// TODO: validate pricing
	// if !rg.Price.IsPositive() {
	// 	return fmt.Errorf("error: invalid unit price (not positive fails)")
	// }

	return limits, nil
}

func validateResourceUnit(config ValConfig, units types.ResourceUnits) (resourceLimits, error) {
	limits := newLimits()

	val, err := validateCPU(config, units.CPU)
	if err != nil {
		return resourceLimits{}, err
	}
	limits.cpu = limits.cpu.Add(val)

	val, err = validateMemory(config, units.Memory)
	if err != nil {
		return resourceLimits{}, err
	}
	limits.memory = limits.memory.Add(val)

	val, err = validateStorage(config, units.Storage)
	if err != nil {
		return resourceLimits{}, err
	}
	limits.storage = limits.storage.Add(val)

	return limits, nil
}

func validateCPU(config ValConfig, u *types.CPU) (sdk.Int, error) {
	if u == nil {
		return sdk.Int{}, errors.Errorf("error: invalid unit cpu, cannot be nil")
	}
	if (u.Units.Value() > uint64(config.MaxUnitCPU)) || (u.Units.Value() < uint64(config.MinUnitCPU)) {
		return sdk.Int{}, errors.Errorf("error: invalid unit cpu (%v > %v > %v fails)",
			config.MaxUnitCPU, u.Units.Value(), config.MinUnitCPU)
	}

	return u.Units.Val, nil
}

func validateMemory(config ValConfig, u *types.Memory) (sdk.Int, error) {
	if u == nil {
		return sdk.Int{}, errors.Errorf("error: invalid unit memory, cannot be nil")
	}
	if (u.Quantity.Value() > uint64(config.MaxUnitMemory)) || (u.Quantity.Value() < uint64(config.MinUnitMemory)) {
		return sdk.Int{}, errors.Errorf("error: invalid unit memory (%v > %v > %v fails)",
			config.MaxUnitMemory, u.Quantity.Value(), config.MinUnitMemory)
	}

	return u.Quantity.Val, nil
}

func validateStorage(config ValConfig, u *types.Storage) (sdk.Int, error) {
	if u == nil {
		return sdk.Int{}, errors.Errorf("error: invalid unit storage, cannot be nil")
	}
	if (u.Quantity.Value() > uint64(config.MaxUnitStorage)) || (u.Quantity.Value() < uint64(config.MinUnitStorage)) {
		return sdk.Int{}, errors.Errorf("error: invalid unit storage (%v > %v > %v fails)",
			config.MaxUnitStorage, u.Quantity.Value(), config.MinUnitStorage)
	}

	return u.Quantity.Val, nil
}
