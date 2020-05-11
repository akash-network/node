package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/crypto/ed25519"

	"github.com/ovrclk/akash/sdl"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

const (
	randDSeq uint64 = 1
	randGSeq uint32 = 2
	randOSeq uint32 = 3
)

func TestToProto(t *testing.T) {
	owner := ed25519.GenPrivKey().PubKey().Address()
	provider := ed25519.GenPrivKey().PubKey().Address()

	leaseID := mtypes.LeaseID{
		Owner:    sdk.AccAddress(owner),
		DSeq:     randDSeq,
		GSeq:     randGSeq,
		OSeq:     randOSeq,
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
		DSeq:     randDSeq,
		GSeq:     randGSeq,
		OSeq:     randOSeq,
		Provider: sdk.AccAddress(provider),
	}
	sdl, err := sdl.ReadFile("../../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	kubeManifest, err := NewManifest("name", leaseID, &mani.GetGroups()[0])
	assert.NoError(t, err)
	t.Logf("kubeManifest: %#v", kubeManifest)

	fromKube := kubeManifest.ManifestGroup()
	rcs := fromKube.GetResources()
	for _, r := range rcs {
		t.Logf("%+v", r)
	}

	assert.Equal(t, fromKube.GetResources()[0].Unit.CPU, uint32(100))
	assert.Equal(t, mani.GetGroups()[0].Name, fromKube.Name)
}
