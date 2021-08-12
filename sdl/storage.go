package sdl

import (
	"sort"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/ovrclk/akash/types"
)

const (
	storageAttributePersistent = "persistent"
	storageAttributeClass      = "class"
	storageAttributeMount      = "mount"
	storageAttributeReadOnly   = "readOnly" // we might not need it at this point of time
)

var (
	errUnsupportedStorageAttribute = errors.New("sdl: unsupported storage attribute")
	errStorageMountPoint           = errors.New("sdl: persistent storage must have mount point")
	errStorageDupMountPoint        = errors.New("sdl: duplicated mount point")
	errStorageMultipleEphemeral    = errors.New("sdl: multiple ephemeral storages are not allowed")
	errStorageDuplicatedVolumeName = errors.New("sdl: duplicated volume name")
)

type v2StorageAttributes types.Attributes
type v2ServiceStorageParams types.Attributes

type v2ResourceStorage struct {
	Name       string              `yaml:"name"`
	Quantity   byteQuantity        `yaml:"size"`
	Attributes v2StorageAttributes `yaml:"attributes,omitempty"`
}

type v2ResourceStorageArray []v2ResourceStorage

var allowedStorageAttributes = map[string]bool{
	storageAttributePersistent: true,
	storageAttributeClass:      true,
}

var allowedServiceStorageAttributes = map[string]bool{
	storageAttributeMount:    true,
	storageAttributeReadOnly: true,
}

// UnmarshalYAML unmarshal storage config
// data can be present either as single entry mapping or an array of them
// e.g
// single entity
// ```yaml
// storage:
//   size: 1Gi
//   attributes:
//     class: ssd
// ```
//
// ```yaml
// storage:
//   - size: 512Mi # ephemeral storage
//   - size: 1Gi
//     name: cache
//     attributes:
//       class: ssd
//   - size: 100Gi
//     name: data
//     attributes:
//       persistent: true # this volumes survives pod restart
//       class: gp # aka general purpose
// ```
func (sdl *v2ResourceStorageArray) UnmarshalYAML(node *yaml.Node) error {
	var nodes v2ResourceStorageArray

	switch node.Kind {
	case yaml.SequenceNode:
		for _, content := range node.Content {
			var nd v2ResourceStorage
			if err := content.Decode(&nd); err != nil {
				return err
			}

			// set default name to ephemeral. later in validation error thrown if multiple
			if nd.Name == "" {
				nd.Name = "ephemeral"
			}
			nodes = append(nodes, nd)
		}
	case yaml.MappingNode:
		var nd v2ResourceStorage
		if err := node.Decode(&nd); err != nil {
			return err
		}

		nd.Name = "ephemeral"
		nodes = append(nodes, nd)
	}

	// check for duplicated volume names
	names := make(map[string]string)
	for _, nd := range nodes {
		if _, exists := names[nd.Name]; exists {
			return errStorageDuplicatedVolumeName
		}

		names[nd.Name] = nd.Name
	}

	nodes.sort()

	*sdl = nodes

	return nil
}

func (sdl *v2StorageAttributes) UnmarshalYAML(node *yaml.Node) error {
	var attr v2StorageAttributes

	var res map[string]string

	if err := node.Decode(&res); err != nil {
		return err
	}

	for k, v := range res {
		if _, validKey := allowedStorageAttributes[k]; !validKey {
			return errors.Wrap(errUnsupportedStorageAttribute, k)
		}

		attr = append(attr, types.Attribute{
			Key:   k,
			Value: v,
		})
	}

	// at this point keys are unique in attributes parsed from sdl so don't need to use sort.SliceStable
	sort.Slice(attr, func(i, j int) bool {
		return attr[i].Key < attr[j].Key
	})

	*sdl = attr

	return nil
}

func (sdl *v2ServiceStorageParams) UnmarshalYAML(node *yaml.Node) error {
	var attr v2ServiceStorageParams

	var res map[string]string

	if err := node.Decode(&res); err != nil {
		return err
	}

	for k, v := range res {
		if _, validKey := allowedServiceStorageAttributes[k]; !validKey {
			return errors.Wrap(errUnsupportedStorageAttribute, k)
		}

		attr = append(attr, types.Attribute{
			Key:   k,
			Value: v,
		})
	}

	// at this point keys are unique in attributes parsed from sdl so don't need to use sort.SliceStable
	sort.Slice(attr, func(i, j int) bool {
		return attr[i].Key < attr[j].Key
	})

	*sdl = attr

	return nil
}

// sort storage slice in the following order
// 1. smaller size
// 2. if sizes are equal then one without mount point goes up
// 3. if both mount points present use lexical order
func (sdl v2ResourceStorageArray) sort() {
	sort.SliceStable(sdl, func(i, j int) bool {
		if sdl[i].Quantity < sdl[j].Quantity {
			return true
		}

		if sdl[i].Quantity > sdl[j].Quantity {
			return false
		}

		if sdl[i].Name < sdl[j].Name {
			return true
		}

		iAttr := types.Attributes(sdl[i].Attributes)
		jAttr := types.Attributes(sdl[j].Attributes)

		iMount, iExist := iAttr.Find(storageAttributeMount).AsString()
		jMount, jExist := jAttr.Find(storageAttributeMount).AsString()

		if !iExist {
			return true
		} else if !jExist {
			return false
		}

		return iMount < jMount
	})
}
