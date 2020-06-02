package testutil

import (
	"testing"

	"github.com/ovrclk/akash/manifest"
)

var (
	// OverflowManifestGenerator generates a manifest maximum integer values
	OverflowManifestGenerator ManifestGenerator = manifestGeneratorOverflow{}
	// RandManifestGenerator generates a manifest with random values
	RandManifestGenerator ManifestGenerator = manifestGeneratorRand{}
	// DefaultManifestGenerator is the default test manifest generator
	DefaultManifestGenerator ManifestGenerator = RandManifestGenerator

	// ManifestGenerators is a list of all available manifest generators
	ManifestGenerators = []struct {
		Name      string
		Generator ManifestGenerator
	}{
		{"overflow", OverflowManifestGenerator},
		{"random", RandManifestGenerator},
	}
)

// ManifestGenerator is an interface for generating test manifests
type ManifestGenerator interface {
	Manifest(t testing.TB) manifest.Manifest
	Group(t testing.TB) manifest.Group
	Service(t testing.TB) manifest.Service
	ServiceExpose(t testing.TB) manifest.ServiceExpose
}
