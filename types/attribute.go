package types

import (
	"reflect"
	"regexp"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"gopkg.in/yaml.v3"
)

const (
	moduleName                = "akash"
	attributeNameRegexpString = `^[a-zA-Z][\w-]{1,30}[a-zA-Z0-9]$`
)

const (
	errAttributesDuplicateKeys uint32 = iota + 1
	errInvalidAttributeKey
)

var (
	ErrAttributesDuplicateKeys = sdkerrors.Register(moduleName, errAttributesDuplicateKeys, "attributes cannot have duplicate keys")
	ErrInvalidAttributeKey     = sdkerrors.Register(moduleName, errInvalidAttributeKey, "attribute key does not match regexp "+attributeNameRegexpString)
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
type AttributeValue string

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

func (m Attribute) AttributeKey() string {
	return m.Key
}

func (m Attribute) AttributeValue() string {
	return m.Key
}

func (attr Attributes) Attributes() []AttributeAccessor {
	result := make([]AttributeAccessor, len(attr))
	for i, v := range attr {
		result[i] = v
	}

	return result
}

func (attr Attributes) Validate() error {
	x := AttributesGetter(attr)
	return ValidateAttributes(x)
}

/*
AttributesSubsetOf check if a is subset of that
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
/**
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

func (attr Attributes) SubsetOf(that Attributes) bool {
	return AttributesSubsetOf(attr, that)
}**/

func AttributesSubsetOf(a, b AttributesGetter) bool {
loop:
	for _, req := range a.Attributes() {
		for _, attr := range b.Attributes() {
			if IsSubsetOf(req, attr) {
				continue loop
			}
		}
		return false
	}

	return true
}

/**
func (attr Attributes) SubsetOf(that Attributes) bool {
	return AttributesSubsetOf(attr, that)
}**/

type AttributeAccessor interface {
	AttributeKey() string
	AttributeValue() string
}

func IsSubsetOf(lhs AttributeAccessor, rhs AttributeAccessor) bool {
	if lhs.AttributeKey() == rhs.AttributeKey() && lhs.AttributeValue() == rhs.AttributeValue() {
		return true
	}

	return false
}

type AttributesGetter interface {
	Attributes() []AttributeAccessor
}

func ValidateAttributes(m AttributesGetter) error {
	store := make(map[string]bool)

	for _, v := range m.Attributes() {
		key := v.AttributeKey()
		if !attributeNameRegexp.MatchString(key) {
			return ErrInvalidAttributeKey
		}

		if _, ok := store[key]; ok {
			return ErrAttributesDuplicateKeys
		}

		store[key] = true
	}

	return nil
}
