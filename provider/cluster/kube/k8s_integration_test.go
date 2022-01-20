//go:build k8s_integration
// +build k8s_integration

package kube

import (
	"bufio"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	"github.com/ovrclk/akash/testutil"
)

func TestNewClient(t *testing.T) {
	// create lease
	lid := testutil.LeaseID(t)
	group := testutil.AppManifestGenerator.Group(t)
	ns := builder.LidNS(lid)

	settings := builder.Settings{
		DeploymentServiceType:          corev1.ServiceTypeClusterIP,
		DeploymentIngressStaticHosts:   false,
		DeploymentIngressDomain:        "bar.com",
		DeploymentIngressExposeLBHosts: false,
	}
	ctx := context.WithValue(context.Background(), builder.SettingsKey, settings)

	ac, err := NewClient(testutil.Logger(t), ns, "")

	require.NoError(t, err)

	cc, ok := ac.(*client)
	require.True(t, ok)
	require.NotNil(t, cc)

	// check inventory
	inventory, err := ac.Inventory(ctx)
	require.NoError(t, err)
	require.NotNil(t, inventory)

	metrics := inventory.Metrics()
	require.Len(t, metrics.Nodes, 1)

	// ensure available nodes
	for _, node := range inventory.Metrics().Nodes {
		require.NotZero(t, node.Available.CPU)
		require.NotZero(t, node.Available.Memory)
	}

	// ensure no deployments
	deployments, err := ac.Deployments(ctx)
	assert.NoError(t, err)
	require.Empty(t, deployments)

	// deploy lease
	err = ac.Deploy(ctx, lid, &group)
	assert.NoError(t, err)

	// query deployments, ensure lease present
	deployments, err = ac.Deployments(ctx)
	require.NoError(t, err)
	require.Len(t, deployments, 1)
	deployment := deployments[0]

	assert.Equal(t, lid, deployment.LeaseID())

	svcname := group.Services[0].Name

	// There is some sort of race here, work around it
	time.Sleep(time.Second * 10)

	lstat, err := ac.LeaseStatus(ctx, lid)
	assert.NoError(t, err)
	assert.Len(t, lstat, 1)
	assert.Equal(t, svcname, lstat[svcname].Name)

	sstat, err := ac.ServiceStatus(ctx, lid, svcname)
	require.NoError(t, err)

	const (
		maxtries = 30
		delay    = time.Second
	)

	tries := 0
	for ; err == nil && tries < maxtries; tries++ {
		t.Log(sstat)
		if uint32(sstat.AvailableReplicas) == group.Services[0].Count {
			break
		}
		time.Sleep(delay)
		sstat, err = ac.ServiceStatus(ctx, lid, svcname)
	}

	assert.NoError(t, err)
	assert.NotEqual(t, maxtries, tries)

	logs, err := ac.LeaseLogs(ctx, lid, svcname, true, nil)
	require.NoError(t, err)
	require.Equal(t, int(sstat.AvailableReplicas), len(logs))

	log := make(chan string, 1)

	go func(scan *bufio.Scanner) {
		for scan.Scan() {
			log <- scan.Text()
			break
		}
	}(logs[0].Scanner)

	select {
	case line := <-log:
		assert.NotEmpty(t, line)
	case <-time.After(10 * time.Second):
		assert.Fail(t, "timed out waiting for logs")
	}

	for _, lg := range logs {
		assert.NoError(t, lg.Stream.Close(), lg.Name)
	}

	npi := cc.kc.NetworkingV1().NetworkPolicies(ns)
	npList, err := npi.List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, len(npList.Items), 0)

	// teardown lease
	err = ac.TeardownLease(ctx, lid)
	assert.NoError(t, err)
}
