package sdl

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

type v2CPUAttributes types.Attributes

type v2ResourceCPU struct {
	Units      cpuQuantity     `yaml:"units"`
	Attributes v2CPUAttributes `yaml:"attributes,omitempty"`
}

func (sdl *v2CPUAttributes) UnmarshalYAML(node *yaml.Node) error {
	var attr v2CPUAttributes

	for i := 0; i+1 < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "arch":
			// Support both string and slice of strings
			var archs []string
			if node.Content[i+1].Kind == yaml.SequenceNode {
				if err := node.Content[i+1].Decode(&archs); err != nil {
					return err
				}
			} else {
				var single string
				if err := node.Content[i+1].Decode(&single); err != nil {
					return err
				}
				archs = append(archs, single)
			}
			for _, value := range archs {
				attr = append(attr, types.Attribute{
					Key:   fmt.Sprintf("capabilities/cpu/arch/%s", value),
					Value: "true",
				})
			}
		default:
			return fmt.Errorf("unsupported cpu attribute \"%s\"", node.Content[i].Value)
		}
	}

	sort.Slice(attr, func(i, j int) bool {
		return attr[i].Key < attr[j].Key
	})

	*sdl = attr

	return nil
}
