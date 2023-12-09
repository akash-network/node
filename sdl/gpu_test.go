package sdl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type testGpuAttributes map[string]string
type testGpuResource struct {
	units gpuQuantity
	attr  testGpuAttributes
}

type gpuTestCase struct {
	name        string
	sdl         string
	expResource testGpuResource
}

func TestV2ResourceGPU_EmptyVendor(t *testing.T) {
	tests := []gpuTestCase{
		{

			name: "missing-vendor",
			sdl: `
units: 1
attributes:
  vendor:
`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var p v2ResourceGPU

			err := yaml.Unmarshal([]byte(test.sdl), &p)
			assert.Error(t, err)
			assert.EqualError(t, err, ErrResourceGPUEmptyVendors.Error())
		})
	}
}

func TestV2ResourceGPU_UnknownVendor(t *testing.T) {
	tests := []gpuTestCase{
		{

			name: "missing-vendor",
			sdl: `
units: 1
attributes:
  vendor:
    foo:
`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var p v2ResourceGPU

			err := yaml.Unmarshal([]byte(test.sdl), &p)
			assert.Error(t, err)
			assert.ErrorContains(t, err, "sdl: unsupported GPU vendor")
		})
	}
}

func TestV2ResourceGPU_InvalidRAM(t *testing.T) {
	tests := []gpuTestCase{
		{

			name: "invalid-ram",
			sdl: `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80G
`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var p v2ResourceGPU

			err := yaml.Unmarshal([]byte(test.sdl), &p)
			assert.Error(t, err)
			assert.EqualError(t, err, errResourceMemoryInvalid.Error())
		})
	}
}

func TestV2ResourceGPU(t *testing.T) {
	tests := []gpuTestCase{
		{
			name: "wildcard-nvidia",
			sdl: `
units: 1
attributes:
  vendor:
    nvidia:
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/nvidia/model/*": "true",
				},
			},
		},
		{
			name: "single-model-nvidia",
			sdl: `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/nvidia/model/a100": "true",
				},
			},
		},
		{
			name: "single-model-with-ram-nvidia",
			sdl: `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80Gi
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/nvidia/model/a100/ram/80Gi": "true",
				},
			},
		},
		{
			name: "multiple-models-with-ram-nvidia",
			sdl: `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80Gi
      - model: a100
        ram: 40Gi
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/nvidia/model/a100/ram/40Gi": "true",
					"vendor/nvidia/model/a100/ram/80Gi": "true",
				},
			},
		},
		{
			name: "multiple-models-mix-nvidia",
			sdl: `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
        ram: 80Gi
      - model: a100
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/nvidia/model/a100":          "true",
					"vendor/nvidia/model/a100/ram/80Gi": "true",
				},
			},
		},
		{
			name: "multiple-models-nvidia",
			sdl: `
units: 1
attributes:
  vendor:
    nvidia:
      - model: a100
      - model: a40
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/nvidia/model/a40":  "true",
					"vendor/nvidia/model/a100": "true",
				},
			},
		},
		{
			name: "multiple-vendors-wildcard",
			sdl: `
units: 1
attributes:
  vendor:
    nvidia:
    amd:
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/nvidia/model/*": "true",
					"vendor/amd/model/*":    "true",
				},
			},
		},
		{
			name: "wildcard-amd",
			sdl: `
units: 1
attributes:
  vendor:
    amd:
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/amd/model/*": "true",
				},
			},
		},
		{
			name: "single-model-amd",
			sdl: `
units: 1
attributes:
  vendor:
    amd:
      - model: mi250
`,
			expResource: testGpuResource{
				units: 1,
				attr: testGpuAttributes{
					"vendor/amd/model/mi250": "true",
				},
			},
		},
	}

	for idx := range tests {
		tc := tests[idx]
		t.Run(tc.name, func(t *testing.T) {
			var p v2ResourceGPU

			err := yaml.Unmarshal([]byte(tc.sdl), &p)
			require.NoError(t, err)

			assert.Equal(t, tc.expResource.units, p.Units)
			assert.Equal(t, len(tc.expResource.attr), len(p.Attributes))

			for i := range p.Attributes {
				assert.Contains(t, tc.expResource.attr, p.Attributes[i].Key)
				assert.Equal(t, tc.expResource.attr[p.Attributes[i].Key], p.Attributes[i].Value)
			}
		})
	}
}
