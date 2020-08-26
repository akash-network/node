package types

// ResourceGroup is the interface that wraps GetName and GetResources methods
type ResourceGroup interface {
	GetName() string
	GetResources() []Resource
}

// Resource stores Unit details and Count value
type Resource struct {
	Unit  Unit
	Count uint32
}

// Equals compare given unit with receiver unit
func (u Unit) Equals(other Unit) bool {
	return u.CPU == other.CPU &&
		u.Memory == other.Memory &&
		u.Storage == other.Storage
}
