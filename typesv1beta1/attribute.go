package typesv1beta1

import (
	"github.com/ovrclk/akash/types"
	"gopkg.in/yaml.v3"
)
import "reflect"

func (m *Attribute) Equal(rhs *Attribute) bool {
	return reflect.DeepEqual(m, rhs)
}

func (m *Attribute) String() string {
	res, _ := yaml.Marshal(m)
	return string(res)
}

func (m Attribute) AttributeKey() string {
	return m.Key
}

func (m Attribute) AttributeValue() string {
	return m.Value
}

type Attributes []Attribute

func (m Attributes) Attributes() []types.AttributeAccessor {
	result := make([]types.AttributeAccessor, len(m))
	for i, v := range m {
		result[i] = v
	}
	return result
}

func (m Attributes) Validate() error {
	x := types.AttributesGetter(m)
	return types.ValidateAttributes(x)
}
