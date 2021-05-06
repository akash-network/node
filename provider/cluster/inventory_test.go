package cluster

import (
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	atypes "github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

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
		{mkres(false, mkrg(1, 1*unit.K, 1*unit.Ki, 4, 1)), false},
		{mkres(false, mkrg(100, 4*unit.G, 98*unit.Gi, 0, 1)), false},
		{mkres(false, mkrg(250, 1*unit.G, 1*unit.Gi, 0, 1)), false},
		{mkres(false, mkrg(1000, 1*unit.G, 1*unit.Gi, 0, 1)), false},
		{mkres(false, mkrg(100, 9*unit.G, 1*unit.Gi, 0, 1)), false},
	}

	externalPortQuantity := uint(3)

	assert.Equal(t, tests[0].ok, reservationAllocateable(inventory, externalPortQuantity, reservations, tests[0].res))
	reservations[0].allocated = true
	reservations[1].allocated = true

	for _, test := range tests[1:] {
		assert.Equal(t, test.ok, reservationAllocateable(inventory, externalPortQuantity, reservations, test.res))
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

	// Shut everything down
	close(donech)
	<-inv.lc.Done()

	// No ports used yet
	require.Equal(t, uint(1000-serviceCount), inv.availableExternalPorts)
}
