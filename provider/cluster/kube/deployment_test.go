package kube

import (
	"os"
	"testing"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tmlibs/log"
)

func TestDeploymentAnnotations(t *testing.T) {
	lease := testutil.Lease(testutil.Address(t), testutil.Address(t), 1, 2, 3)
	mgroup := &types.ManifestGroup{
		Name: "foo",
		Services: []*types.ManifestService{
			{Name: "bar"},
		},
	}

	obj := newDeployment(lease.LeaseID, mgroup)

	anns, err := deploymentToAnnotation(obj)
	require.NoError(t, err)

	robj, err := deploymentFromAnnotation(anns)
	require.NoError(t, err)

	assert.Equal(t, obj, robj)
}

func TestDeploy(t *testing.T) {
	lease := testutil.Lease(testutil.Address(t), testutil.Address(t), 1, 2, 3)

	sdl, err := sdl.ReadFile("../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	// _ := newDeployment(lease.LeaseID, mgroup)

	log := log.NewTMLogger(os.Stdout)
	client, err := NewClient(log)
	assert.NoError(t, err)

	err = client.Deploy(lease.LeaseID, mani.Groups[0])
	assert.NoError(t, err)
}
