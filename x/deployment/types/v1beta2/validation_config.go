package v1beta2

import "github.com/akash-network/node/types/unit"

// This is the validation configuration that acts as a hard limit
// on what the network accepts for deployments. This is never changed
// and is the same across all members of the network

type ValidationConfig struct {

	// MaxUnitCPU is the maximum number of milli (1/1000) cpu units a unit can consume.
	MaxUnitCPU uint
	// MaxUnitMemory is the maximum number of bytes of memory that a unit can consume
	MaxUnitMemory uint64
	// MaxUnitStorage is the maximum number of bytes of storage that a unit can consume
	MaxUnitStorage uint64
	// MaxUnitCount is the maximum number of replias of a service
	MaxUnitCount uint
	// MaxUnitPrice is the maximum price that a unit can have
	MaxUnitPrice uint64

	MinUnitCPU     uint
	MinUnitMemory  uint64
	MinUnitStorage uint64
	MinUnitCount   uint

	// MaxGroupCount is the maximum number of groups allowed per deployment
	MaxGroupCount int
	// MaxGroupUnits is the maximum number services per group
	MaxGroupUnits int

	// MaxGroupCPU is the maximum total amount of CPU requested per group
	MaxGroupCPU uint64
	// MaxGroupMemory is the maximum total amount of memory requested per group
	MaxGroupMemory uint64
	// MaxGroupStorage is the maximum total amount of storage requested per group
	MaxGroupStorage uint64
}

var validationConfig = ValidationConfig{
	MaxUnitCPU:     256 * 1000,    // 256 CPUs
	MaxUnitMemory:  512 * unit.Gi, // 512 Gi
	MaxUnitStorage: 32 * unit.Ti,  // 32 Ti
	MaxUnitCount:   50,
	MaxUnitPrice:   10000000, // 10akt

	MinUnitCPU:     10,
	MinUnitMemory:  unit.Mi,
	MinUnitStorage: 5 * unit.Mi,
	MinUnitCount:   1,

	MaxGroupCount: 20,
	MaxGroupUnits: 20,

	MaxGroupCPU:     512 * 1000,
	MaxGroupMemory:  1024 * unit.Gi,
	MaxGroupStorage: 32 * unit.Ti,
}

func GetValidationConfig() ValidationConfig {
	return validationConfig
}
