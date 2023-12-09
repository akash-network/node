package sdl

import (
	"errors"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

var (
	ErrResourceGPUEmptyVendors = errors.New("sdl: invalid GPU attributes. at least one vendor must be set")
)

type v2GPU struct {
	Model string          `yaml:"model"`
	RAM   *memoryQuantity `yaml:"ram,omitempty"`
}

func (sdl *v2GPU) String() string {
	key := sdl.Model
	if sdl.RAM != nil {
		key += "/ram/" + sdl.RAM.StringWithSuffix("Gi")
	}

	return key
}

type v2GPUs []v2GPU

type gpuVendors map[string]v2GPUs

type v2GPUAttributes types.Attributes

type v2ResourceGPU struct {
	Units      gpuQuantity     `yaml:"units" json:"units"`
	Attributes v2GPUAttributes `yaml:"attributes,omitempty" json:"attributes,omitempty"`
}

func (sdl *v2ResourceGPU) UnmarshalYAML(node *yaml.Node) error {
	res := v2ResourceGPU{}

	for i := 0; i < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "units":
			if err := node.Content[i+1].Decode(&res.Units); err != nil {
				return err
			}
		case "attributes":
			if err := node.Content[i+1].Decode(&res.Attributes); err != nil {
				return err
			}
		default:
			return fmt.Errorf("sdl: unsupported field (%s) for GPU resource", node.Content[i].Value)
		}
	}

	if res.Units > 0 && len(res.Attributes) == 0 {
		return fmt.Errorf("sdl: GPU attributes must be present if units > 0")
	}

	*sdl = res

	return nil
}

func (sdl *v2GPUAttributes) UnmarshalYAML(node *yaml.Node) error {
	var res types.Attributes

	vendors := make(gpuVendors)

	for i := 0; i < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "vendor":
			if err := node.Content[i+1].Decode(&vendors); err != nil {
				return err
			}
		default:
			return fmt.Errorf("sdl: unsupported attribute (%s) for GPU resource", node.Content[i].Value)
		}
	}

	if len(vendors) == 0 {
		return ErrResourceGPUEmptyVendors
	}

	resPrealloc := 0

	for _, models := range vendors {
		if len(models) == 0 {
			resPrealloc++
		} else {
			resPrealloc += len(models)
		}
	}

	for vendor, models := range vendors {
		switch vendor {
		case "nvidia":
		case "amd":
		default:
			return fmt.Errorf("sdl: unsupported GPU vendor (%s)", vendor)
		}

		for _, model := range models {
			res = append(res, types.Attribute{
				Key:   fmt.Sprintf("vendor/%s/model/%s", vendor, model.String()),
				Value: "true",
			})
		}

		if len(models) == 0 {
			res = append(res, types.Attribute{
				Key:   fmt.Sprintf("vendor/%s/model/*", vendor),
				Value: "true",
			})
		}
	}

	sort.Sort(res)

	if err := res.Validate(); err != nil {
		return fmt.Errorf("sdl: invalid GPU attributes: %w", err)
	}

	*sdl = v2GPUAttributes(res)

	return nil
}
