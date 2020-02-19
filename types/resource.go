package types

type ResourceGroup interface {
	GetName() string
	GetResources() []Resource
}

type Resource struct {
	Unit  Unit
	Count uint32
}

type Unit struct {
	CPU     uint32 `json:"cpu"`
	Memory  uint64 `json:"memory"`
	Storage uint64 `json:"storage"`
}

func (u Unit) Equals(other Unit) bool {
	return u.CPU == other.CPU &&
		u.Memory == other.Memory &&
		u.Storage == other.Storage
}
