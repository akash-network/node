package types

import (
	"reflect"

	"gopkg.in/yaml.v3"
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

func (a Attributes) SubsetOf(that Attributes) bool {
	return AttributesSubsetOf(a, that)
}

// type AttributeValue struct {
// 	Val interface{} `json:"value" yaml:"value"`
// }
//
// func NewAttributeValue(val interface{}) AttributeValue {
// 	return AttributeValue{Val: val}
// }
//
// func (a *AttributeValue) Reset() {
// 	a.Val = nil
// }
//
// func (a *AttributeValue) String() string {
// 	res, _ := yaml.Marshal(&a.Val)
// 	return string(res)
// }
//
// func (a *AttributeValue) ProtoMessage() {}
//
// // Marshal implements the gogo proto custom type interface.
// func (a AttributeValue) Marshal() ([]byte, error) {
// 	return nil, nil
// }
//
// // MarshalTo implements the gogo proto custom type interface.
// func (a *AttributeValue) MarshalTo(data []byte) (n int, err error) {
// 	return
// }
//
// // Unmarshal implements the gogo proto custom type interface.
// func (a *AttributeValue) Unmarshal(data []byte) error {
// 	return nil
// }
//
// // Size implements the gogo proto custom type interface.
// func (a *AttributeValue) Size() int {
// 	bz, _ := a.Marshal()
// 	return len(bz)
// }

// func (m Attribute) SubsetOf(rhs Attribute) bool {
// 	if m.Key == rhs.Key {
// 		switch v := m.Value.Val.(type) {
// 		case string:
// 			v2 := rhs.Value.Val.(string)
// 			if v == v2 {
// 				return true
// 			}
// 		case []string:
// 			v2 := rhs.Value.Val.([]string)
// 			// fixme: turn into a func?
// 			for _, mVal := range v {
// 				for _, rhsVal := range v2 {
// 					if mVal == rhsVal {
// 						return true
// 					}
// 				}
// 			}
// 		default:
// 			return false
// 		}
// 	}
//
// 	return false
// }
