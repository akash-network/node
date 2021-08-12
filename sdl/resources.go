package sdl

import (
	"github.com/ovrclk/akash/types"
)

type v2ComputeResources struct {
	CPU     *v2ResourceCPU         `yaml:"cpu"`
	Memory  *v2ResourceMemory      `yaml:"memory"`
	Storage v2ResourceStorageArray `yaml:"storage"`
}

func (sdl *v2ComputeResources) toDGroupResourceUnits() types.ResourceUnits {
	if sdl == nil {
		return types.ResourceUnits{}
	}

	var units types.ResourceUnits
	if sdl.CPU != nil {
		units.CPU = &types.CPU{
			Units:      types.NewResourceValue(uint64(sdl.CPU.Units)),
			Attributes: types.Attributes(sdl.CPU.Attributes),
		}
	}
	if sdl.Memory != nil {
		units.Memory = &types.Memory{
			Quantity:   types.NewResourceValue(uint64(sdl.Memory.Quantity)),
			Attributes: types.Attributes(sdl.Memory.Attributes),
		}
	}

	for _, storage := range sdl.Storage {
		storageEntry := types.Storage{
			Name:       storage.Name,
			Quantity:   types.NewResourceValue(uint64(storage.Quantity)),
			Attributes: types.Attributes(storage.Attributes),
		}

		units.Storage = append(units.Storage, storageEntry)
	}

	return units
}

func toManifestResources(res *v2ComputeResources) types.ResourceUnits {
	var units types.ResourceUnits

	if res.CPU != nil {
		units.CPU = &types.CPU{
			Units: types.NewResourceValue(uint64(res.CPU.Units)),
		}
	}
	if res.Memory != nil {
		units.Memory = &types.Memory{
			Quantity: types.NewResourceValue(uint64(res.Memory.Quantity)),
		}
	}

	for _, storage := range res.Storage {
		storageEntry := types.Storage{
			Name:     storage.Name,
			Quantity: types.NewResourceValue(uint64(storage.Quantity)),
		}

		if storage.Attributes != nil {
			storageEntry.Attributes = make(types.Attributes, len(storage.Attributes))
			copy(storageEntry.Attributes, storage.Attributes)
		}

		units.Storage = append(units.Storage, storageEntry)
	}

	return units
}
