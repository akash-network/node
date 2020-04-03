package kube

import (
	"os"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdl"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
)

func TestDeploy(t *testing.T) {
	t.Skip()

	owner := ed25519.GenPrivKey().PubKey().Address()
	provider := ed25519.GenPrivKey().PubKey().Address()

	leaseID := mtypes.LeaseID{
		Owner:    sdk.AccAddress(owner),
		DSeq:     uint64(1),
		GSeq:     uint32(2),
		OSeq:     uint32(3),
		Provider: sdk.AccAddress(provider),
	}

	sdl, err := sdl.ReadFile("../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	log := log.NewTMLogger(os.Stdout)
	client, err := NewClient(log, "host", "lease")
	assert.NoError(t, err)

	err = client.Deploy(leaseID, &mani.GetGroups()[0])
	assert.NoError(t, err)
}
