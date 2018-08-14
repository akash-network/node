package v1

import (
	"testing"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToProto(t *testing.T) {
	lease := testutil.Lease(testutil.Address(t), testutil.Address(t), 1, 2, 3)

	sdl, err := sdl.ReadFile("../../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, err = NewManifest("name", lease.LeaseID, mani.Groups[0])
	assert.NoError(t, err)
}

func TestFromProto(t *testing.T) {
	lease := testutil.Lease(testutil.Address(t), testutil.Address(t), 1, 2, 3)
	sdl, err := sdl.ReadFile("../../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	kubeManifest, err := NewManifest("name", lease.LeaseID, mani.Groups[0])
	assert.NoError(t, err)

	fromKube := kubeManifest.ManifestGroup()
	assert.NoError(t, err)

	assert.Equal(t, mani.Groups[0].Name, fromKube.Name)
}
