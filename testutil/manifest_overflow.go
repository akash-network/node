package testutil

import (
	"math"
	"testing"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/types"
)

// OverflowManifestGenerator generates a manifest maximum integer values
var OverflowManifestGenerator ManifestGenerator = manifestGeneratorOverflow{}

type manifestGeneratorOverflow struct{}

func (mg manifestGeneratorOverflow) Manifest(t testing.TB) manifest.Manifest {
	t.Helper()
	return []manifest.Group{
		mg.Group(t),
	}
}

func (mg manifestGeneratorOverflow) Group(t testing.TB) manifest.Group {
	t.Helper()
	return manifest.Group{
		Name: "left-coast",
		Services: []manifest.Service{
			mg.Service(t),
		},
	}
}

func (mg manifestGeneratorOverflow) Service(t testing.TB) manifest.Service {
	t.Helper()
	return manifest.Service{
		Name:  "demo",
		Image: "quay.io/ovrclk/demo-app",
		Args:  []string{"run"},
		Env:   []string{"AKASH_TEST_SERVICE=true"},
		Unit: types.Unit{
			CPU:     math.MaxUint32,
			Memory:  math.MaxUint64,
			Storage: math.MaxUint64,
		},
		Count: math.MaxUint32,
		Expose: []manifest.ServiceExpose{
			mg.ServiceExpose(t),
		},
	}
}

func (mg manifestGeneratorOverflow) ServiceExpose(t testing.TB) manifest.ServiceExpose {
	t.Helper()
	return manifest.ServiceExpose{
		Port:         math.MaxUint16,
		ExternalPort: math.MaxUint16,
		Proto:        "http",
		Service:      "svc",
		Global:       true,
		Hosts: []string{
			"foo.com",
		},
	}
}
