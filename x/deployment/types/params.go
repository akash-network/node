package types

import (
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/pkg/errors"
)

// Deployment params default values
const (
	DefaultMaxUnitCPU     uint64 = 1000
	DefaultMaxUnitMemory  uint64 = 1073741824
	DefaultMaxUnitStorage uint64 = 10 * 1073741824
	DefaultMaxUnitCount   uint64 = 10
	DefaultMaxUnitPrice   uint64 = 10000

	DefaultMinUnitCPU     uint64 = 10
	DefaultMinUnitMemory  uint64 = 1024
	DefaultMinUnitStorage uint64 = 1024
	DefaultMinUnitCount   uint64 = 1
	DefaultMinUnitPrice   uint64 = 1

	DefaultMaxGroupCount int64 = 10
	DefaultMaxGroupUnits int64 = 10

	DefaultMaxGroupCPU     int64 = 1000
	DefaultMaxGroupMemory  int64 = 1073741824
	DefaultMaxGroupStorage int64 = 5368709120

	DefaultMinGroupMemPrice int64 = 50
	DefaultMaxGroupMemPrice int64 = 1048576
)

// Parameter store keys
var (
	KeyMaxUnitCPU     = []byte("MaxUnitCPU")
	KeyMaxUnitMemory  = []byte("MaxUnitMemory")
	KeyMaxUnitStorage = []byte("MaxUnitStorage")
	KeyMaxUnitCount   = []byte("MaxUnitCount")
	KeyMaxUnitPrice   = []byte("MaxUnitPrice")

	KeyMinUnitCPU     = []byte("MinUnitCPU")
	KeyMinUnitMemory  = []byte("MinUnitMemory")
	KeyMinUnitStorage = []byte("MinUnitStorage")
	KeyMinUnitCount   = []byte("MinUnitCount")
	KeyMinUnitPrice   = []byte("MinUnitPrice")

	KeyMaxGroupCount = []byte("MaxGroupCount")
	KeyMaxGroupUnits = []byte("MaxGroupUnits")

	KeyMaxGroupCPU     = []byte("MaxGroupCPU")
	KeyMaxGroupMemory  = []byte("MaxGroupMemory")
	KeyMaxGroupStorage = []byte("MaxGroupStorage")

	KeyMinGroupMemPrice = []byte("MinGroupMemPrice")
	KeyMaxGroupMemPrice = []byte("MaxGroupMemPrice")
)

var _ paramtypes.ParamSet = (*Params)(nil)

// ParamTable for staking module
func ParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&Params{})
}

// ParamSetPairs Implements params.ParamSet
func (p *Params) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{
		paramtypes.NewParamSetPair(KeyMaxUnitCPU, &p.MaxUnitCPU, validateUint64),
		paramtypes.NewParamSetPair(KeyMaxUnitMemory, &p.MaxUnitMemory, validateUint64),
		paramtypes.NewParamSetPair(KeyMaxUnitStorage, &p.MaxUnitStorage, validateUint64),
		paramtypes.NewParamSetPair(KeyMaxUnitCount, &p.MaxUnitCount, validateUint64),
		paramtypes.NewParamSetPair(KeyMaxUnitPrice, &p.MaxUnitPrice, validateUint64),

		paramtypes.NewParamSetPair(KeyMinUnitCPU, &p.MinUnitCPU, validateUint64),
		paramtypes.NewParamSetPair(KeyMinUnitMemory, &p.MinUnitMemory, validateUint64),
		paramtypes.NewParamSetPair(KeyMinUnitStorage, &p.MinUnitStorage, validateUint64),
		paramtypes.NewParamSetPair(KeyMinUnitCount, &p.MinUnitCount, validateUint64),
		paramtypes.NewParamSetPair(KeyMinUnitPrice, &p.MinUnitPrice, validateUint64),

		paramtypes.NewParamSetPair(KeyMaxGroupCount, &p.MaxGroupCount, validateInt64),
		paramtypes.NewParamSetPair(KeyMaxGroupUnits, &p.MaxGroupUnits, validateInt64),

		paramtypes.NewParamSetPair(KeyMaxGroupCPU, &p.MaxGroupCPU, validateInt64),
		paramtypes.NewParamSetPair(KeyMaxGroupMemory, &p.MaxGroupMemory, validateInt64),
		paramtypes.NewParamSetPair(KeyMaxGroupStorage, &p.MaxGroupStorage, validateInt64),

		paramtypes.NewParamSetPair(KeyMinGroupMemPrice, &p.MinGroupMemPrice, validateInt64),
		paramtypes.NewParamSetPair(KeyMaxGroupMemPrice, &p.MaxGroupMemPrice, validateInt64),
	}
}

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return Params{
		MaxUnitCPU:       DefaultMaxUnitCPU,
		MaxUnitMemory:    DefaultMaxUnitMemory,
		MaxUnitStorage:   DefaultMaxUnitStorage,
		MaxUnitCount:     DefaultMaxUnitCount,
		MaxUnitPrice:     DefaultMaxUnitPrice,
		MinUnitCPU:       DefaultMinUnitCPU,
		MinUnitMemory:    DefaultMinUnitMemory,
		MinUnitStorage:   DefaultMinUnitStorage,
		MinUnitCount:     DefaultMinUnitCount,
		MinUnitPrice:     DefaultMinUnitPrice,
		MaxGroupCount:    DefaultMaxGroupCount,
		MaxGroupUnits:    DefaultMaxGroupUnits,
		MaxGroupCPU:      DefaultMaxGroupCPU,
		MaxGroupMemory:   DefaultMaxGroupMemory,
		MaxGroupStorage:  DefaultMaxGroupStorage,
		MinGroupMemPrice: DefaultMinGroupMemPrice,
		MaxGroupMemPrice: DefaultMaxGroupMemPrice,
	}
}

// Validate validate a set of params
func (p Params) Validate() error {
	if err := validateUint64(p.MaxUnitCPU); err != nil {
		return err
	}

	if err := validateUint64(p.MaxUnitMemory); err != nil {
		return err
	}

	if err := validateUint64(p.MaxUnitStorage); err != nil {
		return err
	}

	if err := validateUint64(p.MaxUnitCount); err != nil {
		return err
	}

	if err := validateUint64(p.MaxUnitPrice); err != nil {
		return err
	}

	if err := validateUint64(p.MinUnitCPU); err != nil {
		return err
	}

	if err := validateUint64(p.MinUnitMemory); err != nil {
		return err
	}

	if err := validateUint64(p.MinUnitStorage); err != nil {
		return err
	}

	if err := validateUint64(p.MinUnitCount); err != nil {
		return err
	}

	if err := validateUint64(p.MinUnitPrice); err != nil {
		return err
	}

	if err := validateInt64(p.MaxGroupCount); err != nil {
		return err
	}

	if err := validateInt64(p.MaxGroupUnits); err != nil {
		return err
	}

	if err := validateInt64(p.MaxGroupCPU); err != nil {
		return err
	}

	if err := validateInt64(p.MaxGroupMemory); err != nil {
		return err
	}

	if err := validateInt64(p.MaxGroupStorage); err != nil {
		return err
	}

	if err := validateInt64(p.MinGroupMemPrice); err != nil {
		return err
	}

	if err := validateInt64(p.MaxGroupMemPrice); err != nil {
		return err
	}

	return nil
}

func validateUint64(i interface{}) error {
	_, ok := i.(uint64)
	if !ok {
		return errors.Errorf("invalid parameter type: %T", i)
	}

	return nil
}

func validateInt64(i interface{}) error {
	_, ok := i.(int64)
	if !ok {
		return errors.Errorf("invalid parameter type: %T", i)
	}

	return nil
}
