package kube

import (
	"os"
	"testing"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
)

func TestDeploy(t *testing.T) {
	t.Skip()
	lease := testutil.Lease(testutil.Address(t), testutil.Address(t), 1, 2, 3)

	sdl, err := sdl.ReadFile("../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	log := log.NewTMLogger(os.Stdout)
	client, err := NewClient(log, "host", "lease")
	assert.NoError(t, err)

	err = client.Deploy(lease.LeaseID, mani.Groups[0])
	assert.NoError(t, err)
}
