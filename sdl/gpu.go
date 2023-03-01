package sdl

import (
	"sort"

	"gopkg.in/yaml.v3"

	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

type v2GPUAttributes types.Attributes

type v2ResourceGPU struct {
	Units      gpuQuantity     `yaml:"units"`
	Attributes v2CPUAttributes `yaml:"attributes,omitempty"`
}

func (sdl *v2GPUAttributes) UnmarshalYAML(node *yaml.Node) error {
	var attr v2GPUAttributes

	for i := 0; i+1 < len(node.Content); i += 2 {
		var value string
		if err := node.Content[i+1].Decode(&value); err != nil {
			return err
		}
		// switch node.Content[i].Value {
		// case "arch":
		// 	if err := node.Content[i+1].Decode(&value); err != nil {
		// 		return err
		// 	}
		// default:
		// 	return errors.Errorf("unsupported cpu attribute \"%s\"", node.Content[i].Value)
		// }

		attr = append(attr, types.Attribute{
			Key:   node.Content[i].Value,
			Value: value,
		})
	}

	// keys are unique in attributes parsed from sdl so don't need to use sort.SliceStable
	sort.Slice(attr, func(i, j int) bool {
		return attr[i].Key < attr[j].Key
	})

	*sdl = attr

	return nil
}
