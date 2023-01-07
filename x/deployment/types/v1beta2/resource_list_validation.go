package v1beta2

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	types "github.com/akash-network/node/types/v1beta2"
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
	}

	if limits.cpu.GT(sdk.NewIntFromUint64(validationConfig.MaxGroupCPU)) || limits.cpu.LTE(sdk.ZeroInt()) {
		return errors.Errorf("group %v: invalid total CPU (%v > %v > %v fails)",
			rlist.GetName(), validationConfig.MaxGroupCPU, limits.cpu, 0)
	}

	if limits.memory.GT(sdk.NewIntFromUint64(validationConfig.MaxGroupMemory)) || limits.memory.LTE(sdk.ZeroInt()) {
		return errors.Errorf("group %v: invalid total memory (%v > %v > %v fails)",
			rlist.GetName(), validationConfig.MaxGroupMemory, limits.memory, 0)
	}

	for i := range limits.storage {
		if limits.storage[i].GT(sdk.NewIntFromUint64(validationConfig.MaxGroupStorage)) || limits.storage[i].LTE(sdk.ZeroInt()) {
			return errors.Errorf("group %v: invalid total storage (%v > %v > %v fails)",
				rlist.GetName(), validationConfig.MaxGroupStorage, limits.storage, 0)
		}
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

	return limits, nil
}

func validateResourceUnit(units types.ResourceUnits) (resourceLimits, error) {
	limits := newLimits()

	val, err := validateCPU(units.CPU)
	if err != nil {
		return resourceLimits{}, err
	}
	limits.cpu = limits.cpu.Add(val)

	val, err = validateMemory(units.Memory)
	if err != nil {
		return resourceLimits{}, err
	}
	limits.memory = limits.memory.Add(val)

	var storage []sdk.Int
	storage, err = validateStorage(units.Storage)
	if err != nil {
		return resourceLimits{}, err
	}

	// fixme this is not actually sum for storage usecase.
	// do we really need sum here?
	limits.storage = storage

	return limits, nil
}

func validateCPU(u *types.CPU) (sdk.Int, error) {
	if u == nil {
		return sdk.Int{}, errors.Errorf("error: invalid unit CPU, cannot be nil")
	}
	if (u.Units.Value() > uint64(validationConfig.MaxUnitCPU)) || (u.Units.Value() < uint64(validationConfig.MinUnitCPU)) {
		return sdk.Int{}, errors.Errorf("error: invalid unit CPU (%v > %v > %v fails)",
			validationConfig.MaxUnitCPU, u.Units.Value(), validationConfig.MinUnitCPU)
	}

	return u.Units.Val, nil
}

func validateMemory(u *types.Memory) (sdk.Int, error) {
	if u == nil {
		return sdk.Int{}, errors.Errorf("error: invalid unit memory, cannot be nil")
	}
	if (u.Quantity.Value() > validationConfig.MaxUnitMemory) || (u.Quantity.Value() < validationConfig.MinUnitMemory) {
		return sdk.Int{}, errors.Errorf("error: invalid unit memory (%v > %v > %v fails)",
			validationConfig.MaxUnitMemory, u.Quantity.Value(), validationConfig.MinUnitMemory)
	}

	return u.Quantity.Val, nil
}

func validateStorage(u types.Volumes) ([]sdk.Int, error) {
	if u == nil {
		return nil, errors.Errorf("error: invalid unit storage, cannot be nil")
	}

	storage := make([]sdk.Int, 0, len(u))

	for i := range u {
		if (u[i].Quantity.Value() > validationConfig.MaxUnitStorage) || (u[i].Quantity.Value() < validationConfig.MinUnitStorage) {
			return nil, errors.Errorf("error: invalid unit storage (%v > %v > %v fails)",
				validationConfig.MaxUnitStorage, u[i].Quantity.Value(), validationConfig.MinUnitStorage)
		}

		storage = append(storage, u[i].Quantity.Val)
	}

	return storage, nil
}

type resourceLimits struct {
	cpu     sdk.Int
	memory  sdk.Int
	storage []sdk.Int
}

func newLimits() resourceLimits {
	return resourceLimits{
		cpu:    sdk.ZeroInt(),
		memory: sdk.ZeroInt(),
	}
}

func (u *resourceLimits) add(rhs resourceLimits) {
	u.cpu = u.cpu.Add(rhs.cpu)
	u.memory = u.memory.Add(rhs.memory)

	// u.storage = u.storage.Add(rhs.storage)
}

func (u *resourceLimits) mul(count uint32) {
	u.cpu = u.cpu.MulRaw(int64(count))
	u.memory = u.memory.MulRaw(int64(count))
	for i := range u.storage {
		u.storage[i] = u.storage[i].MulRaw(int64(count))
	}
}
