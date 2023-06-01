package sdl

import (
	"fmt"

	"gopkg.in/yaml.v3"

	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

type v2GPUNvidia struct {
	Model string          `yaml:"model"`
	RAM   *memoryQuantity `yaml:"ram,omitempty"`
}

func (sdl *v2GPUNvidia) String() string {
	key := fmt.Sprintf("%s", sdl.Model)
	if sdl.RAM != nil {
		key += "/" + sdl.RAM.StringWithSuffix("Gi")
	}

	return key
}

type v2GPUsNvidia []v2GPUNvidia

type gpuVendor struct {
	Nvidia v2GPUsNvidia `yaml:"nvidia,omitempty"`
}

type v2GPUAttributes struct {
	attr   types.Attributes
	Vendor *gpuVendor `yaml:"vendor,omitempty"`
}

type v2ResourceGPU struct {
	Units      gpuQuantity     `yaml:"units"`
	Attributes v2GPUAttributes `yaml:"attributes,omitempty"`
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

	if res.Units > 0 && len(res.Attributes.attr) == 0 {
		return fmt.Errorf("sdl: GPU attributes must be present if units > 0")
	}

	*sdl = res

	return nil
}

func (sdl *v2GPUAttributes) UnmarshalYAML(node *yaml.Node) error {
	var res v2GPUAttributes

	for i := 0; i < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "vendor":
			if err := node.Content[i+1].Decode(&res.Vendor); err != nil {
				return err
			}
		default:
			return fmt.Errorf("sdl: unsupported attribute (%s) for GPU resource", node.Content[i].Value)
		}
	}

	if res.Vendor == nil {
		return fmt.Errorf("sdl: invalid GPU attributes. at least one vendor must be set")
	}

	res.attr = make(types.Attributes, 0, len(res.Vendor.Nvidia))

	for _, model := range res.Vendor.Nvidia {
		res.attr = append(res.attr, types.Attribute{
			Key:   fmt.Sprintf("vendor/nvidia/model/%s", model.String()),
			Value: "true",
		})
	}

	if len(res.attr) == 0 {
		res.attr = append(res.attr, types.Attribute{
			Key:   "vendor/nvidia/model/*",
			Value: "true",
		})
	}
	res.attr.Sort()

	if err := res.attr.Validate(); err != nil {
		return fmt.Errorf("sdl: invalid GPU attributes: %w", err)
	}

	*sdl = res

	return nil
}
