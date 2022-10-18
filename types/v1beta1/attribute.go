package v1beta1

import (
	"errors"
	"reflect"
	"regexp"

	"gopkg.in/yaml.v3"
)

const (
	attributeNameRegexpString = `^[a-zA-Z][\w-]{1,30}[a-zA-Z0-9]$`
)

var (
	ErrAttributesDuplicateKeys = errors.New("attributes cannot have duplicate keys")
	ErrInvalidAttributeKey     = errors.New("attribute key does not match regexp " + attributeNameRegexpString)
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

// AttributesSubsetOf check if a is subset of that
// For example there are two yaml files being converted into these attributes
// example 1: a is subset of b
// ---
// // a
// // nolint: gofmt
// attributes:
//
//	region:
//	  - us-east-1
//
// ---
// b
// attributes:
//
//	region:
//	  - us-east-1
//	  - us-east-2
//
// example 2: a is not subset of b
// attributes:
//
//	region:
//	  - us-east-1
//
// ---
// b
// attributes:
//
//	region:
//	  - us-east-2
//	  - us-east-3
//
// example 3: a is subset of b
// attributes:
//
//	region:
//	  - us-east-2
//	  - us-east-3
//
// ---
// b
// attributes:
//
//	region:
//	  - us-east-2
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
}
