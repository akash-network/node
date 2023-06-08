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
	key := sdl.Model
	if sdl.RAM != nil {
		key += "/" + sdl.RAM.StringWithSuffix("Gi")
	}

	return key
}

type v2GPUsNvidia []v2GPUNvidia

type gpuVendor struct {
	Nvidia v2GPUsNvidia `yaml:"nvidia,omitempty"`
}

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

	var vendor *gpuVendor

	for i := 0; i < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "vendor":
			if err := node.Content[i+1].Decode(&vendor); err != nil {
				return err
			}
		default:
			return fmt.Errorf("sdl: unsupported attribute (%s) for GPU resource", node.Content[i].Value)
		}
	}

	if vendor == nil {
		return fmt.Errorf("sdl: invalid GPU attributes. at least one vendor must be set")
	}

	res = make(types.Attributes, 0, len(vendor.Nvidia))

	for _, model := range vendor.Nvidia {
		res = append(res, types.Attribute{
			Key:   fmt.Sprintf("vendor/nvidia/model/%s", model.String()),
			Value: "true",
		})
	}

	if len(res) == 0 {
		res = append(res, types.Attribute{
			Key:   "vendor/nvidia/model/*",
			Value: "true",
		})
	}
	res.Sort()

	if err := res.Validate(); err != nil {
		return fmt.Errorf("sdl: invalid GPU attributes: %w", err)
	}

	*sdl = v2GPUAttributes(res)

	return nil
}
