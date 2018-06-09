package bidengine_test

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/provider/bidengine"
	cmocks "github.com/ovrclk/akash/provider/cluster/mocks"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	qmocks "github.com/ovrclk/akash/query/mocks"
	"github.com/ovrclk/akash/testutil"
	txmocks "github.com/ovrclk/akash/txutil/mocks"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := event.NewBus()
	defer bus.Close()

	deployment := testutil.Deployment(testutil.Address(t), 1)
	group := testutil.DeploymentGroups(deployment.Address, 2).Items[0]
	order := testutil.Order(deployment.Address, group.Seq, 3)
	provider := testutil.Provider(testutil.Address(t), 4)

	qclient := new(qmocks.Client)
	qclient.On("DeploymentGroup", mock.Anything, group.DeploymentGroupID).
		Return(group, nil).Once()
	qclient.On("Orders", mock.Anything).
		Return(&types.Orders{}, nil).Once()
	qclient.On("Fulfillment", mock.Anything, mock.Anything).
		Return(nil, nil).Maybe()

	txsent := make(chan struct{})

	txclient := new(txmocks.Client)
	txclient.On("BroadcastTxCommit", mock.Anything).Run(func(args mock.Arguments) {
		defer close(txsent)
		arg, ok := args.Get(0).(*types.TxCreateFulfillment)
		require.True(t, ok)
		require.NotNil(t, arg)

		require.Equal(t, order.OrderID, arg.OrderID())
		require.Equal(t, provider.Address, arg.Provider)

		require.True(t, arg.Price > 0)
	}).Return(nil, nil)

	creso := new(cmocks.Reservation)
	creso.
		On("Group").Return(group).Maybe().
		On("OrderID").Return(order.OrderID).Maybe()

	cluster := new(cmocks.Cluster)
	cluster.
		On("Reserve", order.OrderID, group).Return(creso, nil).Once()

	session := session.New(testutil.Logger(), provider, txclient, qclient)

	service, err := bidengine.NewService(ctx, session, cluster, bus)
	require.NoError(t, err)
	defer service.Close()

	bus.Publish(&event.TxCreateOrder{
		OrderID: order.OrderID,
	})

	select {
	case <-txsent:
	case <-testutil.AfterThreadStart(t):
		assert.Fail(t, "timeout: tx never sent")
	}

	bus.Publish(&event.TxCloseDeployment{
		Deployment: order.Deployment,
	})

	testutil.SleepForThreadStart(t)

	assert.NoError(t, service.Close())

	mock.AssertExpectationsForObjects(t, qclient, txclient, creso, cluster)
}

func TestService_Catchup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := event.NewBus()
	defer bus.Close()

	deployment := testutil.Deployment(testutil.Address(t), 1)
	group := testutil.DeploymentGroups(deployment.Address, 2).Items[0]
	order := testutil.Order(deployment.Address, group.Seq, 3)
	provider := testutil.Provider(testutil.Address(t), 4)

	qclient := new(qmocks.Client)
	qclient.On("DeploymentGroup", mock.Anything, group.DeploymentGroupID).
		Return(group, nil).Once()

	qclient.On("Orders", mock.Anything).
		Return(&types.Orders{
			Items: []*types.Order{order},
		}, nil).Once()

	qclient.On("Fulfillment", mock.Anything, mock.Anything).
		Return(nil, nil).Maybe()

	txsent := make(chan struct{})

	txclient := new(txmocks.Client)
	txclient.On("BroadcastTxCommit", mock.Anything).Run(func(args mock.Arguments) {
		defer close(txsent)
		arg, ok := args.Get(0).(*types.TxCreateFulfillment)
		require.True(t, ok)
		require.NotNil(t, arg)

		require.Equal(t, order.OrderID, arg.OrderID())
		require.Equal(t, provider.Address, arg.Provider)

		require.True(t, arg.Price > 0)
	}).Return(nil, nil)

	creso := new(cmocks.Reservation)
	creso.
		On("Group").Return(group).Maybe().
		On("OrderID").Return(order.OrderID).Maybe()

	cluster := new(cmocks.Cluster)
	cluster.
		On("Reserve", order.OrderID, group).Return(creso, nil).Once()

	session := session.New(testutil.Logger(), provider, txclient, qclient)

	service, err := bidengine.NewService(ctx, session, cluster, bus)
	require.NoError(t, err)
	defer service.Close()

	select {
	case <-txsent:
	case <-testutil.AfterThreadStart(t):
		assert.Fail(t, "timeout: tx never sent")
	}

	testutil.SleepForThreadStart(t)

	assert.NoError(t, service.Close())

	mock.AssertExpectationsForObjects(t, qclient, txclient, creso, cluster)
}
