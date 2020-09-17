package sdl

import (
	"sort"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/ovrclk/akash/types"
)

type v2CPUAttributes types.Attributes

type v2ResourceCPU struct {
	Units      cpuQuantity     `yaml:"units"`
	Attributes v2CPUAttributes `yaml:"attributes,omitempty"`
}

func (sdl *v2CPUAttributes) UnmarshalYAML(node *yaml.Node) error {
	var attr v2CPUAttributes

	for i := 0; i+1 < len(node.Content); i += 2 {
		var value string
		switch node.Content[i].Value {
		case "arch":
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
		default:
			return errors.Errorf("unsupported cpu attribute \"%s\"", node.Content[i].Value)
		}

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
