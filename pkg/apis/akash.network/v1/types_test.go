package v1

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdl"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

func TestToProto(t *testing.T) {
	owner := ed25519.GenPrivKey().PubKey().Address()
	provider := ed25519.GenPrivKey().PubKey().Address()

	leaseID := mtypes.LeaseID{
		Owner:    sdk.AccAddress(owner),
		DSeq:     uint64(1),
		GSeq:     uint32(2),
		OSeq:     uint32(3),
		Provider: sdk.AccAddress(provider),
	}

	sdl, err := sdl.ReadFile("../../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, err = NewManifest("name", leaseID, &mani.GetGroups()[0])
	assert.NoError(t, err)
}

func TestFromProto(t *testing.T) {
	owner := ed25519.GenPrivKey().PubKey().Address()
	provider := ed25519.GenPrivKey().PubKey().Address()

	leaseID := mtypes.LeaseID{
		Owner:    sdk.AccAddress(owner),
		DSeq:     uint64(1),
		GSeq:     uint32(2),
		OSeq:     uint32(3),
		Provider: sdk.AccAddress(provider),
	}
	sdl, err := sdl.ReadFile("../../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	kubeManifest, err := NewManifest("name", leaseID, &mani.GetGroups()[0])
	assert.NoError(t, err)

	fromKube := kubeManifest.ManifestGroup()
	assert.NoError(t, err)

	assert.Equal(t, mani.GetGroups()[0].Name, fromKube.Name)
}
