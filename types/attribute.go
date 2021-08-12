package types

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"gopkg.in/yaml.v3"
)

const (
	moduleName                = "akash"
	attributeNameRegexpString = `^([a-zA-Z][\w\/\.\-]{1,62}\w)$`
)

const (
	errAttributesDuplicateKeys uint32 = iota + 1
	errInvalidAttributeKey
)

var (
	ErrAttributesDuplicateKeys = sdkerrors.Register(moduleName, errAttributesDuplicateKeys, "attributes cannot have duplicate keys")
	ErrInvalidAttributeKey     = sdkerrors.Register(moduleName, errInvalidAttributeKey, "attribute key does not match regexp")
)

var (
	attributeNameRegexp = regexp.MustCompile(attributeNameRegexpString)
)

/*
Attributes purpose of using this type in favor of Cosmos's sdk.Attribute is
ability to later extend it with operators to support querying on things like
cpu/memory/storage attributes
At this moment type though is same as sdk.Attributes but all akash libraries were
turned to use a new one
*/
type Attributes []Attribute

type AttributesGroup []Attributes

type AttributeValue interface {
	AsBool() (bool, bool)
	AsString() (string, bool)
}

type attributeValue struct {
	value string
}

func (val attributeValue) AsBool() (bool, bool) {
	if val.value == "" {
		return false, false
	}

	res, err := strconv.ParseBool(val.value)
	if err != nil {
		return false, false
	}

	return res, true
}

func (val attributeValue) AsString() (string, bool) {
	if val.value == "" {
		return "", false
	}

	return val.value, true
}

func NewStringAttribute(key, val string) Attribute {
	return Attribute{
		Key:   key,
		Value: val,
	}
}

func (m *Attribute) String() string {
	res, _ := yaml.Marshal(m)
	return string(res)
}

func (m *Attribute) Equal(rhs *Attribute) bool {
	return reflect.DeepEqual(m, rhs)
}

func (m Attribute) SubsetOf(rhs Attribute) bool {
	if m.Key == rhs.Key && m.Value == rhs.Value {
		return true
	}

	return false
}

func (attr Attributes) Validate() error {
	store := make(map[string]bool)

	for i := range attr {
		if !attributeNameRegexp.MatchString(attr[i].Key) {
			return ErrInvalidAttributeKey
		}

		if _, ok := store[attr[i].Key]; ok {
			return ErrAttributesDuplicateKeys
		}

		store[attr[i].Key] = true
	}

	return nil
}

func (attr Attributes) Dup() Attributes {
	res := make(Attributes, len(attr))

	for _, pair := range attr {
		res = append(res, Attribute{
			Key:   pair.Key,
			Value: pair.Value,
		})
	}

	return res
}

/*
AttributesSubsetOf check if a is subset of b
For example there are two yaml files being converted into these attributes
example 1: a is subset of b
---
// a
attributes:
  region:
    - us-east-1
---
b
attributes:
  region:
    - us-east-1
    - us-east-2

example 2: a is not subset of b
attributes:
  region:
    - us-east-1
---
b
attributes:
  region:
    - us-east-2
    - us-east-3

example 3: a is subset of b
attributes:
  region:
    - us-east-2
    - us-east-3
---
b
attributes:
  region:
    - us-east-2
*/
func AttributesSubsetOf(a, b Attributes) bool {
loop:
	for _, req := range a {
		for _, attr := range b {
			if req.SubsetOf(attr) {
				continue loop
			}
		}
		return false
	}

	return true
}

func (attr Attributes) SubsetOf(b Attributes) bool {
	return AttributesSubsetOf(attr, b)
}

func (attr Attributes) Find(glob string) AttributeValue {
	// todo wildcard

	var val attributeValue

	for i := range attr {
		if glob == attr[i].Key {
			val.value = attr[i].Value
			break
		}
	}

	return val
}

func (attr Attributes) Iterate(prefix string, fn func(group, key, value string)) {
	for _, item := range attr {
		if strings.HasPrefix(item.Key, prefix) {
			tokens := strings.SplitAfter(item.Key, "/")
			tokens = tokens[1:]
			fn(tokens[1], tokens[2], item.Value)
		}
	}
}

// GetCapabilitiesGroup
//
// example
// capabilities/storage/1/persistent: true
// capabilities/storage/1/class: io1
// capabilities/storage/2/persistent: false
//
// returns
// - - persistent: true
//     class: nvme
// - - persistent: false
func (attr Attributes) GetCapabilitiesGroup(prefix string) AttributesGroup {
	var res AttributesGroup // nolint:prealloc

	groups := make(map[string]Attributes)

	for _, item := range attr {
		if !strings.HasPrefix(item.Key, "capabilities/"+prefix) {
			continue
		}

		tokens := strings.SplitAfter(strings.TrimPrefix(item.Key, "capabilities/"), "/")
		// skip malformed attributes. really?
		if len(tokens) != 3 {
			continue
		}

		// filter out prefix name
		tokens = tokens[1:]

		group := groups[tokens[0]]
		if group == nil {
			group = Attributes{}
		}

		group = append(group, Attribute{
			Key:   tokens[1],
			Value: item.Value,
		})

		groups[tokens[0]] = group
	}

	for _, group := range groups {
		res = append(res, group)
	}

	return res
}

// IN check if given attributes are in attributes group
// AttributesGroup for storage
// - persistent: true
//   class: beta1
// - persistent: true
//   class: beta2
//
// that
// - persistent: true
//   class: beta1
func (attr Attributes) IN(group AttributesGroup) bool {
	for _, group := range group {
		if attr.SubsetOf(group) {
			return true
		}
	}
	return false
}
