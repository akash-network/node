package cluster

import (
	"context"
	"github.com/ovrclk/akash/provider/cluster/operatorclients"
	ipoptypes "github.com/ovrclk/akash/provider/operator/ipoperator/types"
	"github.com/ovrclk/akash/provider/operator/waiter"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types/unit"
	types "github.com/ovrclk/akash/types/v1beta2"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
)

func newInventory(nodes ...string) ctypes.Inventory {
	inv := &inventory{
		nodes: make([]*node, 0, len(nodes)),
		storage: map[string]*storageClassState{
			"beta2": {
				resourcePair: resourcePair{
					allocatable: sdk.NewInt(nullClientStorage),
					allocated:   sdk.NewInt(10 * unit.Gi),
				},
				isDefault: true,
			},
		},
	}

	for _, ndName := range nodes {
		nd := &node{
			id: ndName,
			cpu: resourcePair{
				allocatable: sdk.NewInt(nullClientCPU),
				allocated:   sdk.NewInt(100),
			},
			memory: resourcePair{
				allocatable: sdk.NewInt(nullClientMemory),
				allocated:   sdk.NewInt(1 * unit.Gi),
			},
			ephemeralStorage: resourcePair{
				allocatable: sdk.NewInt(nullClientStorage),
				allocated:   sdk.NewInt(10 * unit.Gi),
			},
		}

		inv.nodes = append(inv.nodes, nd)
	}

	return inv
}

func TestInventory_reservationAllocateable(t *testing.T) {
	mkrg := func(cpu uint64, memory uint64, storage uint64, endpointsCount uint, count uint32) dtypes.Resource {
		endpoints := make([]types.Endpoint, endpointsCount)
		return dtypes.Resource{
			Resources: types.ResourceUnits{
				CPU: &types.CPU{
					Units: types.NewResourceValue(cpu),
				},
				Memory: &types.Memory{
					Quantity: types.NewResourceValue(memory),
				},
				Storage: []types.Storage{
					{
						Quantity: types.NewResourceValue(storage),
					},
				},
				Endpoints: endpoints,
			},
			Count: count,
		}
	}

	mkres := func(allocated bool, res ...dtypes.Resource) *reservation {
		return &reservation{
			allocated: allocated,
			resources: &dtypes.GroupSpec{Resources: res},
		}
	}

	inv := newInventory("a", "b")

	reservations := []*reservation{
		mkres(true, mkrg(750, 3*unit.Gi, 1*unit.Gi, 0, 1)),
		mkres(true, mkrg(100, 4*unit.Gi, 1*unit.Gi, 0, 2)),
		mkres(true, mkrg(2000, 3*unit.Gi, 1*unit.Gi, 0, 2)),
		mkres(true, mkrg(250, 12*unit.Gi, 1*unit.Gi, 0, 2)),
		mkres(true, mkrg(100, 1*unit.G, 1*unit.Gi, 1, 2)),
		mkres(true, mkrg(100, 4*unit.G, 1*unit.Gi, 0, 1)),
		mkres(true, mkrg(100, 4*unit.G, 98*unit.Gi, 0, 1)),
		mkres(true, mkrg(250, 1*unit.G, 1*unit.Gi, 0, 1)),
	}

	for idx, r := range reservations {
		err := inv.Adjust(r)
		require.NoErrorf(t, err, "reservation %d: %v", idx, r)
	}
}

func TestInventory_ClusterDeploymentNotDeployed(t *testing.T) {
	config := Config{
		InventoryResourcePollPeriod:     time.Second,
		InventoryResourceDebugFrequency: 1,
		InventoryExternalPortQuantity:   1000,
	}
	myLog := testutil.Logger(t)
	donech := make(chan struct{})
	bus := pubsub.NewBus()
	subscriber, err := bus.Subscribe()
	require.NoError(t, err)

	deployments := make([]ctypes.Deployment, 0)

	clusterClient := &mocks.Client{}

	clusterInv := newInventory("nodeA")

	clusterClient.On("Inventory", mock.Anything).Return(clusterInv, nil)

	inv, err := newInventoryService(
		config,
		myLog,
		donech,
		subscriber,
		clusterClient,
		operatorclients.NullIPOperatorClient(), // This client is not used in this test
		waiter.NewNullWaiter(),                 // Do not need to wait in test
		deployments)
	require.NoError(t, err)
	require.NotNil(t, inv)

	close(donech)
	<-inv.lc.Done()

	// No ports used yet
	require.Equal(t, uint(1000), inv.availableExternalPorts)
}

func TestInventory_ClusterDeploymentDeployed(t *testing.T) {
	lid := testutil.LeaseID(t)
	config := Config{
		InventoryResourcePollPeriod:     time.Second,
		InventoryResourceDebugFrequency: 1,
		InventoryExternalPortQuantity:   1000,
	}
	myLog := testutil.Logger(t)
	donech := make(chan struct{})
	bus := pubsub.NewBus()
	subscriber, err := bus.Subscribe()
	require.NoError(t, err)

	deployments := make([]ctypes.Deployment, 1)
	deployment := &mocks.Deployment{}
	deployment.On("LeaseID").Return(lid)

	groupServices := make([]manifest.Service, 1)

	serviceCount := testutil.RandRangeInt(1, 10)
	serviceEndpoints := make([]types.Endpoint, serviceCount)

	countOfRandomPortService := testutil.RandRangeInt(0, serviceCount)
	for i := range serviceEndpoints {
		if i < countOfRandomPortService {
			serviceEndpoints[i].Kind = types.Endpoint_RANDOM_PORT
		} else {
			serviceEndpoints[i].Kind = types.Endpoint_SHARED_HTTP
		}
	}

	groupServices[0] = manifest.Service{
		Count: 1,
		Resources: types.ResourceUnits{
			CPU: &types.CPU{
				Units: types.NewResourceValue(1),
			},
			Memory: &types.Memory{
				Quantity: types.NewResourceValue(1 * unit.Gi),
			},
			Storage: []types.Storage{
				{
					Name:     "default",
					Quantity: types.NewResourceValue(1 * unit.Gi),
				},
			},
			Endpoints: serviceEndpoints,
		},
	}
	group := manifest.Group{
		Name:     "nameForGroup",
		Services: groupServices,
	}

	deployment.On("ManifestGroup").Return(group)
	deployments[0] = deployment

	clusterClient := &mocks.Client{}

	clusterInv := newInventory("nodeA")

	inventoryCalled := make(chan int, 1)
	clusterClient.On("Inventory", mock.Anything).Run(func(args mock.Arguments) {
		inventoryCalled <- 0 // Value does not matter
	}).Return(clusterInv, nil)

	inv, err := newInventoryService(
		config,
		myLog,
		donech,
		subscriber,
		clusterClient,
		nil,                    // No IP operator client
		waiter.NewNullWaiter(), // Do not need to wait in test
		deployments)
	require.NoError(t, err)
	require.NotNil(t, inv)

	// Wait for first call to inventory
	<-inventoryCalled

	// Send the event immediately, twice
	// Second version does nothing
	err = bus.Publish(event.ClusterDeployment{
		LeaseID: lid,
		Group: &manifest.Group{
			Name:     "nameForGroup",
			Services: nil,
		},
		Status: event.ClusterDeploymentDeployed,
	})
	require.NoError(t, err)

	err = bus.Publish(event.ClusterDeployment{
		LeaseID: lid,
		Group: &manifest.Group{
			Name:     "nameForGroup",
			Services: nil,
		},
		Status: event.ClusterDeploymentDeployed,
	})
	require.NoError(t, err)

	// Wait for second call to inventory
	<-inventoryCalled

	// wait for cluster deployment to be active
	// needed to avoid data race in reading availableExternalPorts
	for {
		status, err := inv.status(context.Background())
		require.NoError(t, err)

		if len(status.Active) != 0 {
			break
		}

		time.Sleep(time.Second / 2)
	}

	// availableExternalEndpoints should be consumed because of the deployed reservation
	require.Equal(t, uint(1000-countOfRandomPortService), inv.availableExternalPorts)

	// Unreserving the allocated reservation should reclaim the availableExternalEndpoints
	err = inv.unreserve(lid.OrderID())
	require.NoError(t, err)
	require.Equal(t, uint(1000), inv.availableExternalPorts)

	// Shut everything down
	close(donech)
	<-inv.lc.Done()
}

type inventoryScaffold struct {
	leaseIDs        []mtypes.LeaseID
	donech          chan struct{}
	inventoryCalled chan struct{}
	bus             pubsub.Bus
	clusterClient   *mocks.Client
}

func makeInventoryScaffold(t *testing.T, leaseQty uint, inventoryCall bool, nodes ...string) *inventoryScaffold {
	scaffold := &inventoryScaffold{
		donech: make(chan struct{}),
	}

	if inventoryCall {
		scaffold.inventoryCalled = make(chan struct{}, 1)
	}

	for i := uint(0); i != leaseQty; i++ {
		scaffold.leaseIDs = append(scaffold.leaseIDs, testutil.LeaseID(t))
	}

	scaffold.bus = pubsub.NewBus()

	groupServices := make([]manifest.Service, 1)
	serviceCount := testutil.RandRangeInt(1, 50)
	serviceEndpoints := make([]types.Endpoint, serviceCount)

	countOfRandomPortService := testutil.RandRangeInt(0, serviceCount)
	for i := range serviceEndpoints {
		if i < countOfRandomPortService {
			serviceEndpoints[i].Kind = types.Endpoint_RANDOM_PORT
		} else {
			serviceEndpoints[i].Kind = types.Endpoint_SHARED_HTTP
		}
	}

	deploymentRequirements := types.ResourceUnits{
		CPU: &types.CPU{
			Units: types.NewResourceValue(4000),
		},
		Memory: &types.Memory{
			Quantity: types.NewResourceValue(30 * unit.Gi),
		},
		Storage: types.Volumes{
			types.Storage{
				Name:     "default",
				Quantity: types.NewResourceValue((100 * unit.Gi) - 1*unit.Mi),
			},
		},
	}

	deploymentRequirements.Endpoints = serviceEndpoints

	groupServices[0] = manifest.Service{
		Count:     1,
		Resources: deploymentRequirements,
	}

	cclient := &mocks.Client{}

	// Create an inventory set that has enough resources for the deployment
	clusterInv := newInventory(nodes...)

	cclient.On("Inventory", mock.Anything).Run(func(args mock.Arguments) {
		if scaffold.inventoryCalled != nil {
			scaffold.inventoryCalled <- struct{}{}
		}
	}).Return(clusterInv, nil)

	scaffold.clusterClient = cclient

	return scaffold
}

func makeGroupForInventoryTest(sharedHTTP, nodePort, leasedIP bool) manifest.Group {
	groupServices := make([]manifest.Service, 1)

	serviceEndpoints := make([]types.Endpoint, 0)
	seqno := uint32(0)
	if sharedHTTP {
		serviceEndpoint := types.Endpoint{
			Kind:           types.Endpoint_SHARED_HTTP,
			SequenceNumber: seqno,
		}
		serviceEndpoints = append(serviceEndpoints, serviceEndpoint)
	}

	if nodePort {
		serviceEndpoint := types.Endpoint{
			Kind:           types.Endpoint_RANDOM_PORT,
			SequenceNumber: seqno,
		}
		serviceEndpoints = append(serviceEndpoints, serviceEndpoint)
	}

	if leasedIP {
		serviceEndpoint := types.Endpoint{
			Kind:           types.Endpoint_LEASED_IP,
			SequenceNumber: seqno,
		}
		serviceEndpoints = append(serviceEndpoints, serviceEndpoint)
	}

	deploymentRequirements := types.ResourceUnits{
		CPU: &types.CPU{
			Units: types.NewResourceValue(4000),
		},
		Memory: &types.Memory{
			Quantity: types.NewResourceValue(30 * unit.Gi),
		},
		Storage: types.Volumes{
			types.Storage{
				Name:     "default",
				Quantity: types.NewResourceValue((100 * unit.Gi) - 1*unit.Mi),
			},
		},
	}
	deploymentRequirements.Endpoints = serviceEndpoints

	groupServices[0] = manifest.Service{
		Count:     1,
		Resources: deploymentRequirements,
	}
	group := manifest.Group{
		Name:     "nameForGroup",
		Services: groupServices,
	}

	return group
}

func TestInventory_ReserveIPNoIPOperator(t *testing.T) {
	config := Config{
		InventoryResourcePollPeriod:     5 * time.Second,
		InventoryResourceDebugFrequency: 1,
		InventoryExternalPortQuantity:   1000,
	}
	scaffold := makeInventoryScaffold(t, 10, false, "nodeA")
	defer scaffold.bus.Close()

	myLog := testutil.Logger(t)

	subscriber, err := scaffold.bus.Subscribe()
	require.NoError(t, err)
	inv, err := newInventoryService(
		config,
		myLog,
		scaffold.donech,
		subscriber,
		scaffold.clusterClient,
		nil,                    // No IP operator client
		waiter.NewNullWaiter(), // Do not need to wait in test
		make([]ctypes.Deployment, 0))
	require.NoError(t, err)
	require.NotNil(t, inv)

	group := makeGroupForInventoryTest(false, false, true)
	reservation, err := inv.reserve(scaffold.leaseIDs[0].OrderID(), group)
	require.ErrorIs(t, err, errNoLeasedIPsAvailable)
	require.Nil(t, reservation)

	// Shut everything down
	close(scaffold.donech)
	<-inv.lc.Done()
}

func TestInventory_ReserveIPUnavailableWithIPOperator(t *testing.T) {
	config := Config{
		InventoryResourcePollPeriod:     5 * time.Second,
		InventoryResourceDebugFrequency: 1,
		InventoryExternalPortQuantity:   1000,
	}
	scaffold := makeInventoryScaffold(t, 10, false, "nodeA")
	defer scaffold.bus.Close()

	myLog := testutil.Logger(t)

	subscriber, err := scaffold.bus.Subscribe()
	require.NoError(t, err)

	mockIP := &mocks.IPOperatorClient{}

	ipQty := testutil.RandRangeInt(1, 100)
	mockIP.On("GetIPAddressUsage", mock.Anything).Return(ipoptypes.IPAddressUsage{
		Available: uint(ipQty),
		InUse:     uint(ipQty),
	}, nil)
	mockIP.On("Stop")

	inv, err := newInventoryService(
		config,
		myLog,
		scaffold.donech,
		subscriber,
		scaffold.clusterClient,
		mockIP,
		waiter.NewNullWaiter(), // Do not need to wait in test
		make([]ctypes.Deployment, 0))
	require.NoError(t, err)
	require.NotNil(t, inv)

	group := makeGroupForInventoryTest(false, false, true)
	reservation, err := inv.reserve(scaffold.leaseIDs[0].OrderID(), group)
	require.ErrorIs(t, err, errInsufficientIPs)
	require.Nil(t, reservation)

	// Shut everything down
	close(scaffold.donech)
	<-inv.lc.Done()
}

func TestInventory_ReserveIPAvailableWithIPOperator(t *testing.T) {
	config := Config{
		InventoryResourcePollPeriod:     4 * time.Second,
		InventoryResourceDebugFrequency: 1,
		InventoryExternalPortQuantity:   1000,
	}
	scaffold := makeInventoryScaffold(t, 2, false, "nodeA", "nodeB")
	defer scaffold.bus.Close()

	myLog := testutil.Logger(t)

	subscriber, err := scaffold.bus.Subscribe()
	require.NoError(t, err)

	mockIP := &mocks.IPOperatorClient{}

	ipQty := testutil.RandRangeInt(5, 10)
	mockIP.On("GetIPAddressUsage", mock.Anything).Return(ipoptypes.IPAddressUsage{
		Available: uint(ipQty),
		InUse:     uint(ipQty - 1), // not all in use
	}, nil)
	ipAddrStatusCalled := make(chan struct{}, 1)
	// First call indicates no data
	mockIP.On("GetIPAddressStatus", mock.Anything, scaffold.leaseIDs[0].OrderID()).Run(func(args mock.Arguments) {
		ipAddrStatusCalled <- struct{}{}
	}).Return([]ipoptypes.LeaseIPStatus{}, nil).Once()
	// Second call indicates the IP is there and can be confirmed
	mockIP.On("GetIPAddressStatus", mock.Anything, scaffold.leaseIDs[0].OrderID()).Run(func(args mock.Arguments) {
		ipAddrStatusCalled <- struct{}{}
	}).Return([]ipoptypes.LeaseIPStatus{
		{
			Port:         1234,
			ExternalPort: 1234,
			ServiceName:  "foobar",
			IP:           "24.1.2.3",
			Protocol:     "TCP",
		},
	}, nil).Once()

	mockIP.On("Stop")

	inv, err := newInventoryService(
		config,
		myLog,
		scaffold.donech,
		subscriber,
		scaffold.clusterClient,
		mockIP,
		waiter.NewNullWaiter(), // Do not need to wait in test
		make([]ctypes.Deployment, 0))
	require.NoError(t, err)
	require.NotNil(t, inv)

	group := makeGroupForInventoryTest(false, false, true)
	reservation, err := inv.reserve(scaffold.leaseIDs[0].OrderID(), group)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	require.False(t, reservation.Allocated())

	// next reservation fails
	reservation, err = inv.reserve(scaffold.leaseIDs[1].OrderID(), group)
	require.ErrorIs(t, err, errInsufficientIPs)
	require.Nil(t, reservation)

	err = scaffold.bus.Publish(event.ClusterDeployment{
		LeaseID: scaffold.leaseIDs[0],
		Group:   &group,
		Status:  event.ClusterDeploymentDeployed,
	})
	require.NoError(t, err)

	testutil.ChannelWaitForValueUpTo(t, ipAddrStatusCalled, 30*time.Second)
	testutil.ChannelWaitForValueUpTo(t, ipAddrStatusCalled, 30*time.Second)

	// with the 1st reservation confirmed, this one passes now
	reservation, err = inv.reserve(scaffold.leaseIDs[1].OrderID(), group)
	require.NoError(t, err)
	require.NotNil(t, reservation)

	// Shut everything down
	close(scaffold.donech)
	<-inv.lc.Done()

	mockIP.AssertNumberOfCalls(t, "GetIPAddressStatus", 2)
}

func TestInventory_OverReservations(t *testing.T) {
	scaffold := makeInventoryScaffold(t, 10, true, "nodeA")
	defer scaffold.bus.Close()
	lid0 := scaffold.leaseIDs[0]
	lid1 := scaffold.leaseIDs[1]
	myLog := testutil.Logger(t)

	subscriber, err := scaffold.bus.Subscribe()
	require.NoError(t, err)
	defer subscriber.Close()

	groupServices := make([]manifest.Service, 1)

	serviceCount := testutil.RandRangeInt(1, 50)
	serviceEndpoints := make([]types.Endpoint, serviceCount)

	countOfRandomPortService := testutil.RandRangeInt(0, serviceCount)
	for i := range serviceEndpoints {
		if i < countOfRandomPortService {
			serviceEndpoints[i].Kind = types.Endpoint_RANDOM_PORT
		} else {
			serviceEndpoints[i].Kind = types.Endpoint_SHARED_HTTP
		}
	}

	deploymentRequirements := types.ResourceUnits{
		CPU: &types.CPU{
			Units: types.NewResourceValue(4000),
		},
		Memory: &types.Memory{
			Quantity: types.NewResourceValue(30 * unit.Gi),
		},
		Storage: types.Volumes{
			types.Storage{
				Name:     "default",
				Quantity: types.NewResourceValue((100 * unit.Gi) - 1*unit.Mi),
			},
		},
	}
	deploymentRequirements.Endpoints = serviceEndpoints

	groupServices[0] = manifest.Service{
		Count:     1,
		Resources: deploymentRequirements,
	}
	group := manifest.Group{
		Name:     "nameForGroup",
		Services: groupServices,
	}

	config := Config{
		InventoryResourcePollPeriod:     5 * time.Second,
		InventoryResourceDebugFrequency: 1,
		InventoryExternalPortQuantity:   1000,
	}

	inv, err := newInventoryService(
		config,
		myLog,
		scaffold.donech,
		subscriber,
		scaffold.clusterClient,
		nil,                    // No IP operator client
		waiter.NewNullWaiter(), // Do not need to wait in test
		make([]ctypes.Deployment, 0))
	require.NoError(t, err)
	require.NotNil(t, inv)

	// Wait for first call to inventory
	testutil.ChannelWaitForValueUpTo(t, scaffold.inventoryCalled, 30*time.Second)

	// Get the reservation
	reservation, err := inv.reserve(lid0.OrderID(), group)
	require.NoError(t, err)
	require.NotNil(t, reservation)

	// Confirm the second reservation would be too much
	_, err = inv.reserve(lid1.OrderID(), group)
	require.Error(t, err)
	require.ErrorIs(t, err, ctypes.ErrInsufficientCapacity)

	// Send the event immediately to indicate it was deployed
	err = scaffold.bus.Publish(event.ClusterDeployment{
		LeaseID: lid0,
		Group: &manifest.Group{
			Name:     "nameForGroup",
			Services: nil,
		},
		Status: event.ClusterDeploymentDeployed,
	})
	require.NoError(t, err)

	// Give the inventory goroutine time to process the event
	time.Sleep(1 * time.Second)

	// Confirm the second reservation still is too much
	_, err = inv.reserve(lid1.OrderID(), group)
	require.ErrorIs(t, err, ctypes.ErrInsufficientCapacity)

	// Wait for second call to inventory
	testutil.ChannelWaitForValueUpTo(t, scaffold.inventoryCalled, 30*time.Second)

	// // Shut everything down
	close(scaffold.donech)
	<-inv.lc.Done()

	// No ports used yet
	require.Equal(t, uint(1000-countOfRandomPortService), inv.availableExternalPorts)
}
