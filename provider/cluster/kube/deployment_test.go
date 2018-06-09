package kube

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
