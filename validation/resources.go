package validation

import (
	"fmt"

	"github.com/ovrclk/akash/types"
)

func ValidateResourceList(rlist types.ResourceList) error {
	return validateResourceList(defaultConfig, rlist)
}

func validateResourceLists(config config, rlists []types.ResourceList) error {

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

func validateResourceList(config config, rlist types.ResourceList) error {
	if rlist.GetName() == "" {
		return fmt.Errorf("group: empty name")
	}

	units := rlist.GetResources()

	if count := len(units); count > config.MaxGroupUnits {
		return fmt.Errorf("group %v: too many units (%v > %v)",
			rlist.GetName(), count, config.MaxGroupUnits)
	}

	var (
		cpu  int64
		mem  int64
		disk int64
	)

	for _, resource := range units {
		if err := validateResourceGroup(config, resource); err != nil {
			return fmt.Errorf("group %v: %v", rlist.GetName(), err)
		}
		cpu += int64(resource.Unit.CPU * resource.Count)
		mem += int64(resource.Unit.Memory * uint64(resource.Count))
		disk += int64(resource.Unit.Disk * uint64(resource.Count))
	}

	if cpu > config.MaxGroupCPU || cpu <= 0 {
		return fmt.Errorf("group %v: invalid total cpu (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupCPU, cpu, 0)
	}

	if mem > config.MaxGroupMemory || mem <= 0 {
		return fmt.Errorf("group %v: invalid total memory (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupMemory, mem, 0)
	}

	if disk > config.MaxGroupDisk || disk <= 0 {
		return fmt.Errorf("group %v: invalid total disk (%v > %v > %v fails)",
			rlist.GetName(), config.MaxGroupDisk, disk, 0)
	}

	return nil
}

func validateResourceGroup(config config, rg types.ResourceGroup) error {
	if err := validateResourceUnit(config, rg.Unit); err != nil {
		return nil
	}
	if rg.Count > uint32(config.MaxUnitCount) || rg.Count < uint32(config.MinUnitCount) {
		return fmt.Errorf("error: invalid unit count (%v > %v > %v fails)",
			config.MaxUnitCount, rg.Count, config.MinUnitCount)
	}
	return nil
}

func validateResourceUnit(config config, unit types.ResourceUnit) error {
	if unit.CPU > uint32(config.MaxUnitCPU) || unit.CPU < uint32(config.MinUnitCPU) {
		return fmt.Errorf("error: invalide unit cpu (%v > %v > %v fails)",
			config.MaxUnitCPU, unit.CPU, config.MinUnitCPU)
	}
	if unit.Memory > uint64(config.MaxUnitMemory) || unit.Memory < uint64(config.MinUnitMemory) {
		return fmt.Errorf("error: invalid unit memory (%v > %v > %v fails)",
			config.MaxUnitMemory, unit.Memory, config.MinUnitMemory)
	}
	if unit.Disk > uint64(config.MaxUnitDisk) || unit.Disk < uint64(config.MinUnitDisk) {
		return fmt.Errorf("error: invalid unit disk (%v > %v > %v fails)",
			config.MaxUnitDisk, unit.Disk, config.MinUnitDisk)
	}
	return nil
}
