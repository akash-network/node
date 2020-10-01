package sdl

import (
	"github.com/ovrclk/akash/types"
)

type v2ComputeResources struct {
	CPU     *v2ResourceCPU     `yaml:"cpu"`
	Memory  *v2ResourceMemory  `yaml:"memory"`
	Storage *v2ResourceStorage `yaml:"storage"`
}

func (sdl *v2ComputeResources) toResourceUnits() types.ResourceUnits {
	if sdl == nil {
		return types.ResourceUnits{}
	}

	var units types.ResourceUnits
	if sdl.CPU != nil {
		units.CPU = &types.CPU{
			Units:      types.NewResourceValue(uint64(sdl.CPU.Units)),
			Attributes: sdl.CPU.Attributes,
		}
	}
	if sdl.Memory != nil {
		units.Memory = &types.Memory{
			Quantity:   types.NewResourceValue(uint64(sdl.Memory.Quantity)),
			Attributes: sdl.Memory.Attributes,
		}
	}
	if sdl.Storage != nil {
		units.Storage = &types.Storage{
			Quantity:   types.NewResourceValue(uint64(sdl.Storage.Quantity)),
			Attributes: sdl.Storage.Attributes,
		}
	}

	return units
}
