package sdl

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/akash-network/akash-api/go/node/types/unit"
)

var (
	errNegativeValue = fmt.Errorf("invalid: negative value not allowed")
)

var unitSuffixes = map[string]uint64{
	"k":  unit.K,
	"Ki": unit.Ki,
	"M":  unit.M,
	"Mi": unit.Mi,
	"G":  unit.G,
	"Gi": unit.Gi,
	"T":  unit.T,
	"Ti": unit.Ti,
	"P":  unit.P,
	"Pi": unit.Pi,
	"E":  unit.E,
	"Ei": unit.Ei,
}

var memorySuffixes = map[string]uint64{
	"Ki": unit.Ki,
	"Mi": unit.Mi,
	"Gi": unit.Gi,
	"Ti": unit.Ti,
	"Pi": unit.Pi,
	"Ei": unit.Ei,
}

// CPU shares.  One CPUQuantity = 1/1000 of a CPU
type cpuQuantity uint32

type gpuQuantity uint64

func (u *cpuQuantity) UnmarshalYAML(node *yaml.Node) error {
	sval := node.Value
	if strings.HasSuffix(sval, "m") {
		sval = strings.TrimSuffix(sval, "m")
		val, err := strconv.ParseUint(sval, 10, 32)
		if err != nil {
			return err
		}
		*u = cpuQuantity(val)
		return nil
	}

	val, err := strconv.ParseFloat(sval, 64)
	if err != nil {
		return err
	}

	val *= 1000

	if val < 0 {
		return errNegativeValue
	}

	*u = cpuQuantity(val)

	return nil
}

func (u *gpuQuantity) UnmarshalYAML(node *yaml.Node) error {
	sval := node.Value

	val, err := strconv.ParseUint(sval, 10, 64)
	if err != nil {
		return err
	}

	*u = gpuQuantity(val)

	return nil
}

// Memory,Storage size in bytes.
type byteQuantity uint64
type memoryQuantity uint64

func (u *byteQuantity) UnmarshalYAML(node *yaml.Node) error {
	val, err := parseWithSuffix(node.Value, unitSuffixes)
	if err != nil {
		return err
	}
	*u = byteQuantity(val)
	return nil
}

func (u *memoryQuantity) UnmarshalYAML(node *yaml.Node) error {
	val, err := parseWithSuffix(node.Value, memorySuffixes)
	if err != nil {
		return err
	}
	*u = memoryQuantity(val)
	return nil
}

func (u *memoryQuantity) StringWithSuffix(suffix string) string {
	unit, exists := memorySuffixes[suffix]

	val := uint64(*u) / unit

	res := fmt.Sprintf("%d", val)
	if exists {
		res += suffix
	}

	return res
}

func parseWithSuffix(sval string, units map[string]uint64) (uint64, error) {
	for suffix, unit := range units {
		if !strings.HasSuffix(sval, suffix) {
			continue
		}

		sval := strings.TrimSuffix(sval, suffix)

		val, err := strconv.ParseFloat(sval, 64)
		if err != nil {
			return 0, err
		}

		val *= float64(unit)

		if val < 0 {
			return 0, errNegativeValue
		}

		return uint64(val), nil
	}

	val, err := strconv.ParseFloat(sval, 64)
	if err != nil {
		return 0, err
	}

	if val < 0 {
		return 0, errNegativeValue
	}

	return uint64(val), nil
}
