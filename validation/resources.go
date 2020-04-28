package validation

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types"
)

// ValidateResourceList does basic validation for resources list
func ValidateResourceList(rlist types.ResourceGroup) error {
	return validateResourceList(defaultConfig, rlist)
}

func validateResourceLists(config config, rlists []types.ResourceGroup) error {

	if len(rlists) == 0 {
		return fmt.Errorf("error: no groups present")
	}

	if count := len(rlists); count > config.MaxGroupCount {
		return fmt.Errorf("error: too many groups (%v > %v)", count, config.MaxGroupCount)
	}

	names := make(map[string]bool)

	for _, rlist := range rlists {

		if ok := names[rlist.GetName()]; ok {
			return fmt.Errorf("error: duplicate name (%v)", rlist.GetName())
		}
		names[rlist.GetName()] = true

		if err := validateResourceList(config, rlist); err != nil {
			return err
		}
	}
	return nil
}

func validateResourceList(config config, rlist types.ResourceGroup) error {
	if rlist.GetName() == "" {
		return fmt.Errorf("group: empty name")
	}

	units := rlist.GetResources()

	if count := len(units); count > config.MaxGroupUnits {
		return fmt.Errorf("group %v: too many units (%v > %v)",
			rlist.GetName(), count, config.MaxGroupUnits)
	}

	var (
		cpu     = sdk.ZeroUint()
		mem     = sdk.ZeroUint()
		storage = sdk.ZeroUint()
	)

	for _, resource := range units {

		if err := validateResourceGroup(config, resource); err != nil {
			return fmt.Errorf("group %v: %v", rlist.GetName(), err)
		}

		cpu = cpu.Add(sdk.NewUint(uint64(resource.Unit.CPU)).MulUint64(uint64(resource.Count)))
		mem = mem.Add(sdk.NewUint(resource.Unit.Memory).MulUint64(uint64(resource.Count)))
		storage = storage.Add(sdk.NewUint(resource.Unit.Storage).MulUint64(uint64(resource.Count)))

		// TODO: validate pricing
		// if idx == 0 {
		// 	price = resource.Price
		// } else {
		// 	if resource.Price.Denom != price.Denom {
		// 		return fmt.Errorf("mixed denonimations: (%v != %v)", price.Denom, resource.Price.Denom)
		// 	}
		// }
	}

	if cpu.GT(sdk.NewUint(uint64(config.MaxGroupCPU))) || cpu.LTE(sdk.ZeroUint()) {
		return fmt.Errorf("group %v: invalid total cpu (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupCPU, cpu, 0)
	}

	if mem.GT(sdk.NewUint(uint64(config.MaxGroupMemory))) || mem.LTE(sdk.ZeroUint()) {
		return fmt.Errorf("group %v: invalid total memory (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupMemory, mem, 0)
	}

	if storage.GT(sdk.NewUint(uint64(config.MaxGroupStorage))) || storage.LTE(sdk.ZeroUint()) {
		return fmt.Errorf("group %v: invalid total disk (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupStorage, storage, 0)
	}

	return nil
}

func validateResourceGroup(config config, rg types.Resource) error {
	if err := validateResourceUnit(config, rg.Unit); err != nil {
		return nil
	}
	if rg.Count > uint32(config.MaxUnitCount) || rg.Count < uint32(config.MinUnitCount) {
		return fmt.Errorf("error: invalid unit count (%v > %v > %v fails)",
			config.MaxUnitCount, rg.Count, config.MinUnitCount)
	}

	// TODO: validate pricing
	// if !rg.Price.IsPositive() {
	// 	return fmt.Errorf("error: invalid unit price (not positive fails)")
	// }

	return nil
}

func validateResourceUnit(config config, unit types.Unit) error {
	if unit.CPU > uint32(config.MaxUnitCPU) || unit.CPU < uint32(config.MinUnitCPU) {
		return fmt.Errorf("error: invalide unit cpu (%v > %v > %v fails)",
			config.MaxUnitCPU, unit.CPU, config.MinUnitCPU)
	}
	if unit.Memory > uint64(config.MaxUnitMemory) || unit.Memory < uint64(config.MinUnitMemory) {
		return fmt.Errorf("error: invalid unit memory (%v > %v > %v fails)",
			config.MaxUnitMemory, unit.Memory, config.MinUnitMemory)
	}
	if unit.Storage > uint64(config.MaxUnitStorage) || unit.Storage < uint64(config.MinUnitStorage) {
		return fmt.Errorf("error: invalid unit disk (%v > %v > %v fails)",
			config.MaxUnitStorage, unit.Storage, config.MinUnitStorage)
	}
	return nil
}
