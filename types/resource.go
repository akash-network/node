package types

import (
	"reflect"
)

type UnitType int

type Unit interface {
	String() string
	equals(Unit) bool
	add(Unit) error
	Sub(Unit) error
	le(Unit) bool
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

// Add it rather searches for existing entry of the same type and sums values
// if type not found it appends
func (m ResourceUnits) Add(rhs ResourceUnits) (ResourceUnits, error) {
	res := m

	if res.CPU != nil {
		if err := res.CPU.add(rhs.CPU); err != nil {
			return ResourceUnits{}, err
		}
	} else {
		res.CPU = rhs.CPU
	}

	if res.Memory != nil {
		if err := res.Memory.add(rhs.Memory); err != nil {
			return ResourceUnits{}, err
		}
	} else {
		res.Memory = rhs.Memory
	}

	// if res.Storage != nil {
	// 	if err := res.Storage.add(rhs.Storage); err != nil {
	// 		return ResourceUnits{}, err
	// 	}
	// } else {
	// 	res.Storage = rhs.Storage
	// }

	return res, nil
}

// Sub tbd
func (m ResourceUnits) Sub(rhs ResourceUnits) (ResourceUnits, error) {
	if (m.CPU == nil && rhs.CPU != nil) ||
		(m.Memory == nil && rhs.Memory != nil) ||
		(m.Storage == nil && rhs.Storage != nil) {
		return ResourceUnits{}, ErrCannotSub
	}

	// Make a deep copy
	res := ResourceUnits{
		CPU:    &CPU{},
		Memory: &Memory{},
		// Storage:   &Storage{},
		Endpoints: nil,
	}
	*res.CPU = *m.CPU
	*res.Memory = *m.Memory
	// *res.Storage = *m.Storage
	res.Endpoints = make([]Endpoint, len(m.Endpoints))
	copy(res.Endpoints, m.Endpoints)

	if res.CPU != nil {
		if err := res.CPU.Sub(rhs.CPU); err != nil {
			return ResourceUnits{}, err
		}
	}
	if res.Memory != nil {
		if err := res.Memory.Sub(rhs.Memory); err != nil {
			return ResourceUnits{}, err
		}
	}

	// if res.Storage != nil {
	// 	if err := res.Storage.sub(rhs.Storage); err != nil {
	// 		return ResourceUnits{}, err
	// 	}
	// }

	return res, nil
}

func (m ResourceUnits) Equals(rhs ResourceUnits) bool {
	return reflect.DeepEqual(m, rhs)
}

func (m CPU) Dup() *CPU {
	return &CPU{
		Units:      m.Units.Dup(),
		Attributes: m.Attributes.Dup(),
	}
}

func (m *CPU) equals(other Unit) bool {
	rhs, valid := other.(*CPU)
	if !valid {
		return false
	}

	if !m.Units.equals(rhs.Units) || len(m.Attributes) != len(rhs.Attributes) {
		return false
	}

	return reflect.DeepEqual(m.Attributes, rhs.Attributes)
}

func (m *CPU) le(other Unit) bool {
	rhs, valid := other.(*CPU)
	if !valid {
		return false
	}

	return m.Units.le(rhs.Units)
}

func (m *CPU) add(other Unit) error {
	rhs, valid := other.(*CPU)
	if !valid {
		return nil
	}

	res, err := m.Units.add(rhs.Units)
	if err != nil {
		return err
	}

	m.Units = res

	return nil
}

func (m *CPU) Sub(other Unit) error {
	rhs, valid := other.(*CPU)
	if !valid {
		return nil
	}

	res, err := m.Units.sub(rhs.Units)
	if err != nil {
		return err
	}

	m.Units = res

	return nil
}

func (m Memory) Dup() *Memory {
	return &Memory{
		Quantity:   m.Quantity.Dup(),
		Attributes: m.Attributes.Dup(),
	}
}

func (m *Memory) equals(other Unit) bool {
	rhs, valid := other.(*Memory)
	if !valid {
		return false
	}

	if !m.Quantity.equals(rhs.Quantity) || len(m.Attributes) != len(rhs.Attributes) {
		return false
	}

	return reflect.DeepEqual(m.Attributes, rhs.Attributes)
}

func (m *Memory) le(other Unit) bool {
	rhs, valid := other.(*Memory)
	if !valid {
		return false
	}

	return m.Quantity.le(rhs.Quantity)
}

func (m *Memory) add(other Unit) error {
	rhs, valid := other.(*Memory)
	if !valid {
		return nil
	}

	res, err := m.Quantity.add(rhs.Quantity)
	if err != nil {
		return err
	}

	m.Quantity = res

	return nil
}

func (m *Memory) Sub(other Unit) error {
	rhs, valid := other.(*Memory)
	if !valid {
		return nil
	}

	res, err := m.Quantity.sub(rhs.Quantity)
	if err != nil {
		return err
	}

	m.Quantity = res

	return nil
}

func (m Storage) Dup() *Storage {
	return &Storage{
		Quantity:   m.Quantity.Dup(),
		Attributes: m.Attributes.Dup(),
	}
}

func (m *Storage) equals(other Unit) bool {
	rhs, valid := other.(*Storage)
	if !valid {
		return false
	}

	if !m.Quantity.equals(rhs.Quantity) || len(m.Attributes) != len(rhs.Attributes) {
		return false
	}

	return reflect.DeepEqual(m.Attributes, rhs.Attributes)
}

func (m *Storage) le(other Unit) bool {
	rhs, valid := other.(*Storage)
	if !valid {
		return false
	}

	return m.Quantity.le(rhs.Quantity)
}

func (m *Storage) add(other Unit) error {
	rhs, valid := other.(*Storage)
	if !valid {
		return nil
	}

	res, err := m.Quantity.add(rhs.Quantity)
	if err != nil {
		return err
	}

	m.Quantity = res

	return nil
}

func (m *Storage) Sub(other Unit) error {
	rhs, valid := other.(*Storage)
	if !valid {
		return nil
	}

	res, err := m.Quantity.sub(rhs.Quantity)
	if err != nil {
		return err
	}

	m.Quantity = res

	return nil
}

func (m Volumes) Equal(rhs Volumes) bool {
	return reflect.DeepEqual(m, rhs)
}

func (m Volumes) Dup() Volumes {
	res := make(Volumes, len(m))

	for _, storage := range m {
		res = append(res, *storage.Dup())
	}

	return res
}
