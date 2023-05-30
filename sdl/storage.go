package sdl

import (
	"errors"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

const (
	StorageEphemeral           = "ephemeral"
	StorageAttributePersistent = "persistent"
	StorageAttributeClass      = "class"
	StorageAttributeMount      = "mount"
	StorageAttributeReadOnly   = "readOnly" // we might not need it at this point of time
	StorageClassDefault        = "default"
)

var (
	errUnsupportedStorageAttribute  = errors.New("sdl: unsupported storage attribute")
	errStorageDupMountPoint         = errors.New("sdl: duplicated mount point")
	errStorageMultipleRootEphemeral = errors.New("sdl: multiple root ephemeral storages are not allowed")
	errStorageDuplicatedVolumeName  = errors.New("sdl: duplicated volume name")
	errStorageEphemeralClass        = errors.New("sdl: ephemeral storage should not set attribute class")
)

type v2StorageAttributes types.Attributes

type v2ServiceStorageParams struct {
	Mount    string `yaml:"mount"`
	ReadOnly bool   `yaml:"readOnly"`
}

type v2ResourceStorage struct {
	Name       string              `yaml:"name"`
	Quantity   byteQuantity        `yaml:"size"`
	Attributes v2StorageAttributes `yaml:"attributes,omitempty"`
}

type v2ResourceStorageArray []v2ResourceStorage

type validateAttrFn func(string, *string) error

var allowedStorageClasses = map[string]bool{
	"default": true,
	"beta1":   true,
	"beta2":   true,
	"beta3":   true,
}

var validateStorageAttributes = map[string]validateAttrFn{
	StorageAttributePersistent: validateAttributeBool,
	StorageAttributeClass:      validateAttributeStorageClass,
}

func validateAttributeBool(key string, val *string) error {
	if res, valid := unifyStringAsBool(*val); valid {
		*val = res

		return nil
	}

	return fmt.Errorf("sdl: invalid value for attribute \"%s\". expected bool", key)
}

func validateAttributeStorageClass(_ string, val *string) error {
	if _, valid := allowedStorageClasses[*val]; valid {
		return nil
	}

	return fmt.Errorf("sdl: invalid value for attribute class")
}

// UnmarshalYAML unmarshal storage config
// data can be present either as single entry mapping or an array of them
// nolint: gofmt
// e.g
// single entity
// ```yaml
// storage:
//
//	size: 1Gi
//	attributes:
//	  class: ssd
//
// ```
//
// ```yaml
// storage:
//   - size: 512Mi # ephemeral storage
//   - size: 1Gi
//     name: cache
//     attributes:
//     class: ssd
//   - size: 100Gi
//     name: data
//     attributes:
//     persistent: true # this volumes survives pod restart
//     class: gp # aka general purpose
//
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
				nd.Name = "default"
			}
			nodes = append(nodes, nd)
		}
	case yaml.MappingNode:
		var nd v2ResourceStorage
		if err := node.Decode(&nd); err != nil {
			return err
		}

		nd.Name = "default"
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

	// set default
	if _, set := res[StorageAttributePersistent]; !set {
		res[StorageAttributePersistent] = valueFalse
	}

	persistent := res[StorageAttributePersistent]
	class := res[StorageAttributeClass]
	if persistent == valueFalse && class != "" {
		return errStorageEphemeralClass
	}
	if persistent == valueTrue && class == "" {
		res[StorageAttributeClass] = StorageClassDefault
	}

	for k, v := range res {
		validateFn, supportedAttr := validateStorageAttributes[k]
		if !supportedAttr {
			return fmt.Errorf("%w: %s", errUnsupportedStorageAttribute, k)
		}

		val := v
		if err := validateFn(k, &val); err != nil {
			return err
		}

		attr = append(attr, types.Attribute{
			Key:   k,
			Value: val,
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
// 2. if sizes are equal then one without class goes up
// 3. when both class present use lexicographic order
// 4. if no class in both cases check persistent attribute. one persistent = false goes up
// 5. volume name
func (sdl v2ResourceStorageArray) sort() {
	sort.SliceStable(sdl, func(i, j int) bool {
		if sdl[i].Quantity < sdl[j].Quantity {
			return true
		}

		if sdl[i].Quantity > sdl[j].Quantity {
			return false
		}

		iAttr := types.Attributes(sdl[i].Attributes)
		jAttr := types.Attributes(sdl[j].Attributes)

		iClass, iExists := iAttr.Find(StorageAttributePersistent).AsString()
		jClass, jExists := jAttr.Find(StorageAttributePersistent).AsString()

		if (!iExists && jExists) ||
			(jExists && iExists && iClass < jClass) {
			return true
		} else if iExists && !jExists {
			return false
		}

		iPersistent, _ := iAttr.Find(StorageAttributePersistent).AsBool()
		jPersistent, _ := jAttr.Find(StorageAttributePersistent).AsBool()

		if !iPersistent {
			return true
		} else if !jPersistent {
			return false
		}

		return sdl[i].Name < sdl[j].Name
	})
}
