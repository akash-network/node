package sdl

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ovrclk/akash/types/unit"
)

var (
	errNegativeValue = fmt.Errorf("invalid: negative value not allowed")
)

var unitSuffixes = []struct {
	symbol string
	unit   uint64
}{
	{"k", unit.K},
	{"Ki", unit.Ki},

	{"M", unit.M},
	{"Mi", unit.Mi},

	{"G", unit.G},
	{"Gi", unit.Gi},

	{"T", unit.T},
	{"Ti", unit.Ti},

	{"P", unit.P},
	{"Pi", unit.Pi},

	{"E", unit.E},
	{"Ei", unit.Ei},
}

// CPU shares.  One CPUQuantity = 1/1000 of a CPU
type cpuQuantity uint32

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

// Memory,Storage size in bytes.
type byteQuantity uint64

func (u *byteQuantity) UnmarshalYAML(node *yaml.Node) error {
	val, err := parseWithSuffix(node.Value)
	if err != nil {
		return err
	}
	*u = byteQuantity(val)
	return nil
}

func parseWithSuffix(sval string) (uint64, error) {
	for _, suffix := range unitSuffixes {
		if !strings.HasSuffix(sval, suffix.symbol) {
			continue
		}

		sval := strings.TrimSuffix(sval, suffix.symbol)

		val, err := strconv.ParseFloat(sval, 64)
		if err != nil {
			return 0, err
		}

		val *= float64(suffix.unit)

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
