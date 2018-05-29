package cluster_test

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	log := testutil.Logger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := event.NewBus()
	defer bus.Close()

	c, err := cluster.NewService(log, ctx, bus)
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
