package cluster

import (
	"testing"
	"time"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	atypes "github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

func newResourceUnits() types.ResourceUnits {
	return types.ResourceUnits{
		CPU:     &types.CPU{Units: types.NewResourceValue(1000)},
		Memory:  &types.Memory{Quantity: types.NewResourceValue(10 * unit.Gi)},
		Storage: &types.Storage{Quantity: types.NewResourceValue(100 * unit.Gi)},
	}
}

func zeroResourceUnits() types.ResourceUnits {
	return types.ResourceUnits{
		CPU:     &types.CPU{Units: types.NewResourceValue(0)},
		Memory:  &types.Memory{Quantity: types.NewResourceValue(0 * unit.Gi)},
		Storage: &types.Storage{Quantity: types.NewResourceValue(0 * unit.Gi)},
	}
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
				Storage: &types.Storage{
					Quantity: types.NewResourceValue(storage),
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

	inventory := []ctypes.Node{
		NewNode("a", newResourceUnits(), newResourceUnits()),
		NewNode("b", newResourceUnits(), newResourceUnits()),
	}

	reservations := []*reservation{
		mkres(false, mkrg(750, 3*unit.Gi, 1*unit.Gi, 0, 1)),
		mkres(false, mkrg(100, 4*unit.Gi, 1*unit.Gi, 0, 2)),
		mkres(true, mkrg(2000, 3*unit.Gi, 1*unit.Gi, 0, 2)),
		mkres(true, mkrg(250, 12*unit.Gi, 1*unit.Gi, 0, 2)),
	}

	tests := []struct {
		res *reservation
		ok  bool // Determines if the allocation should be allocatable or not
	}{
		{mkres(false, mkrg(100, 1*unit.G, 1*unit.Gi, 1, 2)), true},
		{mkres(false, mkrg(100, 4*unit.G, 1*unit.Gi, 0, 1)), true},
		{mkres(false, mkrg(20001, 1*unit.K, 1*unit.Ki, 4, 1)), false},
		{mkres(false, mkrg(100, 4*unit.G, 98*unit.Gi, 0, 1)), true},
		{mkres(false, mkrg(250, 1*unit.G, 1*unit.Gi, 0, 1)), true},
		{mkres(false, mkrg(1000, 1*unit.G, 201*unit.Gi, 0, 1)), false},
		{mkres(false, mkrg(100, 21*unit.Gi, 1*unit.Gi, 0, 1)), false},
	}

	externalPortQuantity := uint(3)

	for i, test := range tests {
		assert.Equalf(t, test.ok, reservationAllocateable(inventory, externalPortQuantity, reservations, test.res), "test %d", i)

		if i == 0 {
			reservations[0].allocated = true
			reservations[1].allocated = true
		}
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
	result := make([]ctypes.Node, 0)
	clusterClient.On("Inventory", mock.Anything).Return(result, nil)

	inv, err := newInventoryService(
		config,
		myLog,
		donech,
		subscriber,
		clusterClient,
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
	serviceEndpoints := make([]atypes.Endpoint, serviceCount)
	groupServices[0] = manifest.Service{
		Count: 1,
		Resources: atypes.ResourceUnits{
			CPU: &atypes.CPU{
				Units: types.NewResourceValue(1),
			},
			Memory: &atypes.Memory{
				Quantity: types.NewResourceValue(1 * unit.Gi),
			},
			Storage: &atypes.Storage{
				Quantity: types.NewResourceValue(1 * unit.Gi),
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
	result := make([]ctypes.Node, 0)
	inventoryCalled := make(chan int, 1)
	clusterClient.On("Inventory", mock.Anything).Run(func(args mock.Arguments) {
		inventoryCalled <- 0 // Value does not matter
	}).Return(result, nil)

	inv, err := newInventoryService(
		config,
		myLog,
		donech,
		subscriber,
		clusterClient,
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

	// availableExternalEndpoints should be consumed because of the deployed reservation
	require.Equal(t, uint(1000-serviceCount), inv.availableExternalPorts)

	// Unreserving the allocated reservation should reclaim the availableExternalEndpoints
	err = inv.unreserve(lid.OrderID())
	require.NoError(t, err)
	require.Equal(t, uint(1000), inv.availableExternalPorts)

	// Shut everything down
	close(donech)
	<-inv.lc.Done()
}

func TestInventory_OverReservations(t *testing.T) {
	lid0 := testutil.LeaseID(t)
	lid1 := testutil.LeaseID(t)

	config := Config{
		InventoryResourcePollPeriod:     5 * time.Second,
		InventoryResourceDebugFrequency: 1,
		InventoryExternalPortQuantity:   1000,
	}

	myLog := testutil.Logger(t)
	donech := make(chan struct{})
	bus := pubsub.NewBus()
	subscriber, err := bus.Subscribe()
	require.NoError(t, err)

	groupServices := make([]manifest.Service, 1)

	serviceCount := testutil.RandRangeInt(1, 10)
	serviceEndpoints := make([]atypes.Endpoint, serviceCount)

	deploymentRequirements, err := newResourceUnits().Sub(
		atypes.ResourceUnits{
			CPU: &types.CPU{Units: types.NewResourceValue(1)},
			Memory: &atypes.Memory{
				Quantity: types.NewResourceValue(1 * unit.Mi),
			},
			Storage: &atypes.Storage{
				Quantity: types.NewResourceValue(1 * unit.Mi),
			},
			Endpoints: []atypes.Endpoint{},
		})
	require.NoError(t, err)
	deploymentRequirements.Endpoints = serviceEndpoints

	groupServices[0] = manifest.Service{
		Count:     1,
		Resources: deploymentRequirements,
	}
	group := manifest.Group{
		Name:     "nameForGroup",
		Services: groupServices,
	}

	deployment := &mocks.Deployment{}
	deployment.On("ManifestGroup").Return(group)
	deployment.On("LeaseID").Return(lid1)

	clusterClient := &mocks.Client{}

	inventoryCalled := make(chan int, 1)

	// Create an inventory set that has enough resources for the deployment
	result := make([]ctypes.Node, 1)

	result[0] = NewNode("testnode", newResourceUnits(), newResourceUnits())
	inventoryUpdates := make(chan ctypes.Node, 1)
	clusterClient.On("Inventory", mock.Anything).Run(func(args mock.Arguments) {
		select {
		case newNode := <-inventoryUpdates:
			result[0] = newNode
		default:
			// don't block
		}

		inventoryCalled <- 0 // Value does not matter
	}).Return(result, nil)

	inv, err := newInventoryService(
		config,
		myLog,
		donech,
		subscriber,
		clusterClient,
		make([]ctypes.Deployment, 0))
	require.NoError(t, err)
	require.NotNil(t, inv)

	// Wait for first call to inventory
	<-inventoryCalled

	// Get the reservation
	reservation, err := inv.reserve(lid0.OrderID(), deployment.ManifestGroup())
	require.NoError(t, err)
	require.NotNil(t, reservation)

	// Confirm the second reservation would be too much
	_, err = inv.reserve(lid1.OrderID(), deployment.ManifestGroup())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInsufficientCapacity)

	// Send the event immediately to indicate it was deployed
	err = bus.Publish(event.ClusterDeployment{
		LeaseID: lid0,
		Group: &manifest.Group{
			Name:     "nameForGroup",
			Services: nil,
		},
		Status: event.ClusterDeploymentDeployed,
	})
	require.NoError(t, err)

	// The the cluster mock that the reported inventory has changed
	inventoryUpdates <- NewNode("testNode", newResourceUnits(), zeroResourceUnits())

	// Give the inventory goroutine time to process the event
	time.Sleep(1 * time.Second)

	// Confirm the second reservation still is too much
	_, err = inv.reserve(lid1.OrderID(), deployment.ManifestGroup())
	require.ErrorIs(t, err, ErrInsufficientCapacity)

	// Wait for second call to inventory
	<-inventoryCalled

	// Shut everything down
	close(donech)
	<-inv.lc.Done()

	// No ports used yet
	require.Equal(t, uint(1000-serviceCount), inv.availableExternalPorts)
}
