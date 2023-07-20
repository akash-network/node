package sdl

import (
	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

type v2ComputeResources struct {
	CPU     *v2ResourceCPU         `yaml:"cpu"`
	GPU     *v2ResourceGPU         `yaml:"gpu"`
	Memory  *v2ResourceMemory      `yaml:"memory"`
	Storage v2ResourceStorageArray `yaml:"storage"`
}

func (sdl *v2ComputeResources) toResources() types.Resources {
	if sdl == nil {
		return types.Resources{}
	}

	units := types.Resources{
		Endpoints: types.Endpoints{},
	}

	if sdl.CPU != nil {
		units.CPU = &types.CPU{
			Units:      types.NewResourceValue(uint64(sdl.CPU.Units)),
			Attributes: types.Attributes(sdl.CPU.Attributes),
		}
	}

	if sdl.GPU != nil {
		units.GPU = &types.GPU{
			Units:      types.NewResourceValue(uint64(sdl.GPU.Units)),
			Attributes: types.Attributes(sdl.GPU.Attributes),
		}
	} else {
		units.GPU = &types.GPU{
			Units: types.NewResourceValue(0),
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
