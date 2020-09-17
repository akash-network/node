package sdl

import (
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/ovrclk/akash/types"
)

type v2StorageAttributes types.Attributes

type v2ResourceStorage struct {
	Quantity   byteQuantity        `yaml:"size"`
	Attributes v2StorageAttributes `yaml:"attributes,omitempty"`
}

func (sdl *v2StorageAttributes) UnmarshalYAML(node *yaml.Node) error {
	var attr v2StorageAttributes

	var res map[string]string

	if err := node.Decode(&res); err != nil {
		return err
	}

	for k, v := range res {
		attr = append(attr, types.Attribute{
			Key:   k,
			Value: v,
		})
	}

	// keys are unique in attributes parsed from sdl so don't need to use sort.SliceStable
	sort.Slice(attr, func(i, j int) bool {
		return attr[i].Key < attr[j].Key
	})

	*sdl = attr

	return nil
}
