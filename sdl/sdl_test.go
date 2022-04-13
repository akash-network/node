package sdl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSDLManifestVersion(t *testing.T) {
	obj, err := ReadFile("_testdata/simple.yaml")
	require.NoError(t, err)

	m, err := obj.Manifest()
	require.NoError(t, err)

	version, err := ManifestVersion(m)
	require.NoError(t, err)
	// Should return a value
	require.NotEmpty(t, version)

	obj, err = ReadFile("_testdata/private_service.yaml")
	require.NoError(t, err)

	m, err = obj.Manifest()
	require.NoError(t, err)

	secondVersion, err := ManifestVersion(m)
	require.NoError(t, err)
	// Should return a value
	require.NotEmpty(t, secondVersion)
	// Should be different than the first
	require.NotEqual(t, secondVersion, version)
}

func TestSDLManifestVersionChangesWithVersion(t *testing.T) {
	obj, err := ReadFile("_testdata/simple.yaml")
	require.NoError(t, err)

	m, err := obj.Manifest()
	require.NoError(t, err)

	version, err := ManifestVersion(m)
	require.NoError(t, err)
	// Should return a value
	require.NotEmpty(t, version)

	obj, err = ReadFile("_testdata/simple-double-ram.yaml")
	require.NoError(t, err)

	m, err = obj.Manifest()
	require.NoError(t, err)

	secondVersion, err := ManifestVersion(m)
	require.NoError(t, err)
	// Should return a value
	require.NotEmpty(t, secondVersion)
	// Should be different than the first
	require.NotEqual(t, secondVersion, version)
}
