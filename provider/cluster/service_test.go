package cluster_test

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_Reserve(t *testing.T) {
	log := testutil.Logger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := event.NewBus()
	defer bus.Close()

	c, err := cluster.NewService(log, ctx, bus, cluster.NullClient())
	require.NoError(t, err)

	group := testutil.DeploymentGroups(testutil.DeploymentAddress(t), 1).Items[0]
	order := testutil.Order(group.DeploymentID(), group.Seq, 1)

	reservation, err := c.Reserve(order.OrderID, group)
	require.NoError(t, err)

	assert.Equal(t, order.OrderID, reservation.OrderID())
	assert.Equal(t, group, reservation.Group())

	require.NoError(t, c.Close())

	_, err = c.Reserve(order.OrderID, group)
	assert.Error(t, err, cluster.ErrNotRunning)
}

func TestService_Teardown_TxCloseDeployment(t *testing.T) {
	withServiceTestSetup(t, func(bus event.Bus, leaseID types.LeaseID) {
		err := bus.Publish(&event.TxCloseDeployment{
			Deployment: leaseID.Deployment,
		})
		require.NoError(t, err)
	})
}

func TestService_Teardown_TxCloseFulfillment(t *testing.T) {
	withServiceTestSetup(t, func(bus event.Bus, leaseID types.LeaseID) {
		err := bus.Publish(&event.TxCloseFulfillment{
			FulfillmentID: leaseID.FulfillmentID(),
		})
		require.NoError(t, err)
	})
}

func withServiceTestSetup(t *testing.T, fn func(event.Bus, types.LeaseID)) {

	log := testutil.Logger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := event.NewBus()
	defer bus.Close()

	deployment := testutil.Deployment(testutil.DeploymentAddress(t), 1)

	group := testutil.DeploymentGroups(deployment.Address, 2).Items[0]
	order := testutil.Order(deployment.Address, group.Seq, 3)

	lease := testutil.Lease(testutil.Address(t), order.Deployment, order.Group, order.Seq, 10)

	manifest := &types.Manifest{
		Groups: []*types.ManifestGroup{
			{
				Name: group.Name,
			},
		},
	}

	client := new(mocks.Client)

	client.On("Deploy", order.OrderID, manifest.Groups[0]).
		Return(nil).
		Once()

	client.On("Teardown", order.OrderID).
		Return(nil).
		Once()

	c, err := cluster.NewService(log, ctx, bus, client)
	require.NoError(t, err)

	_, err = c.Reserve(order.OrderID, group)
	require.NoError(t, err)

	err = bus.Publish(event.ManifestReceived{
		LeaseID:    lease.LeaseID,
		Manifest:   manifest,
		Deployment: deployment,
		Group:      group,
	})
	require.NoError(t, err)

	testutil.SleepForThreadStart(t)

	fn(bus, lease.LeaseID)
	testutil.SleepForThreadStart(t)

	require.NoError(t, c.Close())
	mock.AssertExpectationsForObjects(t, client)
}
