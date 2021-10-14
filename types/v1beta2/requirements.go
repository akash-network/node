package v1beta2

import (
	"gopkg.in/yaml.v3"
)

func (m *SignedBy) String() string {
	res, _ := yaml.Marshal(m)
	return string(res)
}

func (m *PlacementRequirements) String() string {
	res, _ := yaml.Marshal(m)
	return string(res)
}
