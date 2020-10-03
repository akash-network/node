package sdl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/ovrclk/akash/types/unit"
)

func TestCPUQuantity(t *testing.T) {

	type vtype struct {
		Val cpuQuantity `yaml:"val"`
	}

	tests := []struct {
		text  string
		value uint32
		err   bool
	}{
		{`val: 1`, 1000, false},
		{`val: -1`, 1000, true},

		{`val: 0.5`, 500, false},
		{`val: -0.5`, 500, true},

		{`val: "100m"`, 100, false},
		{`val: "-100m"`, 100, true},

		{`val: ""`, 0, true},
	}

	for idx, test := range tests {
		buf := []byte(test.text)
		obj := &vtype{}

		err := yaml.Unmarshal(buf, obj)

		if test.err {
			assert.Error(t, err, "idx:%v text:`%v`", idx, test.text)
			continue
		}

		if !assert.NoError(t, err, "idx:%v text:`%v`", idx, test.text) {
			continue
		}

		assert.Equal(t, cpuQuantity(test.value), obj.Val, "idx:%v text:`%v`", idx, test.text)
	}
}

func TestByteQuantity(t *testing.T) {
	type vtype struct {
		Val byteQuantity `yaml:"val"`
	}

	tests := []struct {
		text  string
		value uint64
		err   bool
	}{
		{`val: 1`, 1, false},
		{`val: -1`, 1, true},

		{`val: "1M"`, unit.M, false},
		{`val: "-1M"`, 0, true},

		{`val: "0.5M"`, unit.M / 2, false},
		{`val: "-0.5M"`, 0, true},

		{`val: "3M"`, 3 * unit.M, false},
		{`val: "3G"`, 3 * unit.G, false},
		{`val: "3T"`, 3 * unit.T, false},
		{`val: "3P"`, 3 * unit.P, false},
		{`val: "3E"`, 3 * unit.E, false},

		{`val: ""`, 0, true},
	}

	for idx, test := range tests {
		buf := []byte(test.text)
		obj := &vtype{}

		err := yaml.Unmarshal(buf, obj)

		if test.err {
			assert.Error(t, err, "idx:%v text:`%v`", idx, test.text)
			continue
		}

		if !assert.NoError(t, err, "idx:%v text:`%v`", idx, test.text) {
			continue
		}

		assert.Equal(t, byteQuantity(test.value), obj.Val, "idx:%v text:`%v`", idx, test.text)
	}
}
