package types


// This is the validation configuration that acts as a hard limit
// on what the network accepts for deployments. This is never changed
// and is the same across all members of the network

var validationConfig = struct {
	MaxUnitCPU uint
	MaxUnitMemory  uint
	MaxUnitStorage uint
	MaxUnitCount   uint
	MaxUnitPrice   uint

	MinUnitCPU     uint
	MinUnitMemory  uint
	MinUnitStorage uint
	MinUnitCount   uint
	MinUnitPrice   uint

	MaxGroupCount int
	MaxGroupUnits int

	MaxGroupCPU     int64
	MaxGroupMemory  int64
	MaxGroupStorage int64

	MinGroupMemPrice int64
	MaxGroupMemPrice int64
}{
	MaxUnitCPU: 500,
	MaxUnitMemory:  1073741824,
	MaxUnitStorage: 1073741824,
	MaxUnitCount:  10,
	MaxUnitPrice:   10000,

	MinUnitCPU:     10,
	MinUnitMemory:  1024,
	MinUnitStorage: 1024,
	MinUnitCount:  1,
	MinUnitPrice:   1,

	MaxGroupCount: 10,
	MaxGroupUnits: 10,

	MaxGroupCPU:     1000,
	MaxGroupMemory:  1073741824,
	MaxGroupStorage: 5368709120,

	MinGroupMemPrice: 50,
	MaxGroupMemPrice: 1048576,
}
