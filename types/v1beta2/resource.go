package v1beta2

type UnitType int

type Unit interface {
	String() string
}

type ResUnit interface {
	Equals(ResUnit) bool
	Add(unit ResUnit) bool
}

// Resources stores Unit details and Count value
type Resources struct {
	Resources ResourceUnits `json:"resources"`
	Count     uint32        `json:"count"`
}

// ResourceGroup is the interface that wraps GetName and GetResources methods
type ResourceGroup interface {
	GetName() string
	GetResources() []Resources
}

type Volumes []Storage

var _ Unit = (*CPU)(nil)
var _ Unit = (*Memory)(nil)
var _ Unit = (*Storage)(nil)

func (m ResourceUnits) Dup() ResourceUnits {
	res := ResourceUnits{
		CPU:       m.CPU.Dup(),
		Memory:    m.Memory.Dup(),
		Storage:   m.Storage.Dup(),
		Endpoints: m.Endpoints.Dup(),
	}

	return res
}

func (m CPU) Dup() *CPU {
	return &CPU{
		Units:      m.Units.Dup(),
		Attributes: m.Attributes.Dup(),
	}
}

func (m Memory) Dup() *Memory {
	return &Memory{
		Quantity:   m.Quantity.Dup(),
		Attributes: m.Attributes.Dup(),
	}
}

func (m Storage) Dup() *Storage {
	return &Storage{
		Quantity:   m.Quantity.Dup(),
		Attributes: m.Attributes.Dup(),
	}
}

func (m Volumes) Equal(rhs Volumes) bool {
	for i := range m {
		if !m[i].Equal(rhs[i]) {
			return false
		}
	}

	return true
}

func (m Volumes) Dup() Volumes {
	res := make(Volumes, len(m))

	for _, storage := range m {
		res = append(res, *storage.Dup())
	}

	return res
}
