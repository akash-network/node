package cluster_test

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	qmocks "github.com/ovrclk/akash/query/mocks"
	"github.com/ovrclk/akash/testutil"
	txumocks "github.com/ovrclk/akash/txutil/mocks"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService_Reserve(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := event.NewBus()
	defer bus.Close()

	session := providerSession(t)

	c, err := cluster.NewService(ctx, session, bus, cluster.NullClient())
	require.NoError(t, err)
	testutil.WaitReady(t, c.Ready())

	group := testutil.DeploymentGroup(testutil.DeploymentAddress(t), 1)
	order := testutil.Order(group.DeploymentID(), group.Seq, 1)

	reservation, err := c.Reserve(order.OrderID, group)
	require.NoError(t, err)

	assert.Equal(t, order.OrderID, reservation.OrderID())
	assert.Equal(t, group, reservation.Resources())

	require.NoError(t, c.Close())

	_, err = c.Reserve(order.OrderID, group)
	assert.Equal(t, cluster.ErrNotRunning, err)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := event.NewBus()
	defer bus.Close()

	deployment := testutil.Deployment(testutil.DeploymentAddress(t), 1)

	group := testutil.DeploymentGroup(deployment.Address, 2)
	order := testutil.Order(deployment.Address, group.Seq, 3)

	lease := testutil.Lease(testutil.Address(t), order.Deployment, order.Group, order.Seq, 10)

	manifest := &types.Manifest{
		Groups: []*types.ManifestGroup{
			{
				Name: group.Name,
				Services: []*types.ManifestService{
					{
						Unit:  &group.Resources[0].Unit,
						Count: group.Resources[0].Count,
					},
				},
			},
		},
	}

	client := new(mocks.Client)

	client.On("Deploy", lease.LeaseID, manifest.Groups[0]).
		Return(nil).
		Once()

	client.On("TeardownLease", lease.LeaseID).
		Return(nil).
		Once()

	client.On("Deployments").
		Return(nil, nil).
		Once()

	client.On("LeaseStatus", lease.LeaseID).
		Return(&types.LeaseStatusResponse{}, nil).
		Maybe()

	client.On("Inventory").
		Return(cluster.NullClient().Inventory()).
		Maybe()

	c, err := cluster.NewService(ctx, providerSession(t), bus, client)
	require.NoError(t, err)
	testutil.WaitReady(t, c.Ready())

	testutil.SleepForThreadStart(t)

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

func providerSession(t *testing.T) session.Session {
	log := testutil.Logger()
	txc := new(txumocks.Client)
	qc := new(qmocks.Client)
	provider := testutil.Provider(testutil.Address(t), 1)
	return session.New(log, provider, txc, qc)
}
