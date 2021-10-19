package cluster

import (
	"context"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
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

	deployment := &mocks.Deployment{}
	deployment.On("ManifestGroup").Return(group)
	deployment.On("LeaseID").Return(lid1)

	clusterClient := &mocks.Client{}

	inventoryCalled := make(chan int, 1)

	// Create an inventory set that has enough resources for the deployment
	clusterInv := newInventory("nodeA")

	clusterClient.On("Inventory", mock.Anything).Run(func(args mock.Arguments) {
		inventoryCalled <- 0 // Value does not matter
	}).Return(clusterInv, nil)

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
	require.ErrorIs(t, err, ctypes.ErrInsufficientCapacity)

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

	// Give the inventory goroutine time to process the event
	time.Sleep(1 * time.Second)

	// Confirm the second reservation still is too much
	_, err = inv.reserve(lid1.OrderID(), deployment.ManifestGroup())
	require.ErrorIs(t, err, ctypes.ErrInsufficientCapacity)

	// Wait for second call to inventory
	<-inventoryCalled

	// // Shut everything down
	close(donech)
	<-inv.lc.Done()

	// No ports used yet
	require.Equal(t, uint(1000-countOfRandomPortService), inv.availableExternalPorts)
}
