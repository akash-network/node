package validation

import (
	"fmt"

	"github.com/ovrclk/akash/types"
)

func validateResourceLists(config config, rlists []types.ResourceList) error {

	if len(rlists) == 0 {
		return fmt.Errorf("error: no groups present")
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

func validateResourceList(config config, rlist types.ResourceList) error {
	if rlist.GetName() == "" {
		return fmt.Errorf("group: empty name")
	}
	for _, resource := range rlist.GetResources() {
		if err := validateResourceGroup(config, resource); err != nil {
			return fmt.Errorf("group %v: %v", rlist.GetName(), err)
		}
	}
	return nil
}

func validateResourceGroup(config config, rg types.ResourceGroup) error {
	if err := validateResourceUnit(config, rg.Unit); err != nil {
		return nil
	}
	if rg.Count > uint32(config.MaxUnitCount) {
		return fmt.Errorf("error: unit count too high (%v > %v).", rg.Count, config.MaxUnitCount)
	}
	if rg.Price > uint64(config.MaxUnitPrice) {
		return fmt.Errorf("error: unit price too high (%v > %v).", rg.Price, config.MaxUnitPrice)
	}
	return nil
}

func validateResourceUnit(config config, unit types.ResourceUnit) error {
	if unit.CPU > uint32(config.MaxUnitCPU) {
		return fmt.Errorf("error: unit cpu too high (%v > %v).", unit.CPU, config.MaxUnitCPU)
	}
	if unit.Memory > uint64(config.MaxUnitMemory) {
		return fmt.Errorf("error: unit memory too high (%v > %v).", unit.Memory, config.MaxUnitMemory)
	}
	if unit.Disk > uint64(config.MaxUnitDisk) {
		return fmt.Errorf("error: unit disk too high (%v > %v).", unit.Disk, config.MaxUnitDisk)
	}
	return nil
}
