// +build k8s_integration

package kube

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestNewClient(t *testing.T) {
	ctx := context.Background()

	ns := fmt.Sprintf("test-provider-cluster-kube-client-%v", rand.Uint32())

	settings := settings{
		DeploymentServiceType:          corev1.ServiceTypeClusterIP,
		DeploymentIngressStaticHosts:   false,
		DeploymentIngressDomain:        "bar.com",
		DeploymentIngressExposeLBHosts: false,
	}

	client, err := newClientWithSettings(testutil.Logger(t), "localhost", ns, settings)
	require.NoError(t, err)

	// check inventory
	nodes, err := client.Inventory(ctx)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	// ensure available nodes
	node := nodes[0]
	require.NotZero(t, node.Available().CPU)
	require.NotZero(t, node.Available().Memory)

	// ensure no deployments
	deployments, err := client.Deployments(ctx)
	assert.NoError(t, err)
	require.Empty(t, deployments)

	// create lease
	lid := testutil.LeaseID(t)
	group := testutil.AppManifestGenerator.Group(t)

	// deploy lease
	err = client.Deploy(ctx, lid, &group)
	assert.NoError(t, err)

	// query deployments, ensure lease present
	deployments, err = client.Deployments(ctx)
	require.NoError(t, err)
	require.Len(t, deployments, 1)
	deployment := deployments[0]

	assert.Equal(t, lid, deployment.LeaseID())

	svcname := group.Services[0].Name

	lstat, err := client.LeaseStatus(ctx, lid)
	assert.NoError(t, err)
	assert.Len(t, lstat.Services, 1)
	assert.Equal(t, svcname, lstat.Services[0].Name)

	sstat, err := client.ServiceStatus(ctx, lid, svcname)
	require.NoError(t, err)

	const (
		maxtries = 10
		delay    = time.Second
	)

	tries := 0
	for ; err == nil && tries < maxtries; tries++ {
		t.Log(sstat)
		if uint32(sstat.AvailableReplicas) == group.Services[0].Count {
			break
		}
		time.Sleep(delay)
		sstat, err = client.ServiceStatus(ctx, lid, svcname)
	}

	assert.NoError(t, err)
	assert.NotEqual(t, maxtries, tries)

	// ensure inventory used
	// XXX: not working with kind. might be a delay issue?
	// curnodes, err := client.Inventory(ctx)
	// require.NoError(t, err)
	// require.Len(t, curnodes, 1)
	// curnode := curnodes[0]
	// assert.Less(t, node.Available().CPU, curnode.Available().CPU)
	// assert.Less(t, node.Available().Memory, curnode.Available().Memory)

	// teardown lease
	err = client.TeardownLease(ctx, lid)
	assert.NoError(t, err)
}
