package cluster

import (
	"github.com/boz/go-lifecycle"
	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMonitorInstantiate(t *testing.T) {
	myLog := testutil.Logger(t)
	bus := pubsub.NewBus()
	lid := testutil.LeaseID(t)

	group := &manifest.Group{}
	client := &mocks.Client{}

	statusResult := &ctypes.LeaseStatus{}
	client.On("LeaseStatus", mock.Anything, lid).Return(statusResult, nil)
	mySession := session.New(myLog, nil, nil, -1)

	lc := lifecycle.New()
	myDeploymentManager := &deploymentManager{
		bus:     bus,
		session: mySession,
		client:  client,
		lease:   lid,
		mgroup:  group,
		log:     myLog,
		lc:      lc,
	}
	monitor := newDeploymentMonitor(myDeploymentManager)
	require.NotNil(t, monitor)

	monitor.lc.Shutdown(nil)
}

func TestMonitorSendsClusterDeploymentPending(t *testing.T) {
	const serviceName = "test"
	myLog := testutil.Logger(t)
	bus := pubsub.NewBus()
	lid := testutil.LeaseID(t)

	group := &manifest.Group{}
	group.Services = make([]manifest.Service, 1)
	group.Services[0].Name = serviceName
	group.Services[0].Expose = make([]manifest.ServiceExpose, 1)
	group.Services[0].Expose[0].ExternalPort = 2000
	group.Services[0].Expose[0].Proto = manifest.TCP
	group.Services[0].Expose[0].Port = 40000
	client := &mocks.Client{}

	statusResult := make(map[string]*ctypes.ServiceStatus)
	client.On("LeaseStatus", mock.Anything, lid).Return(statusResult, nil)
	mySession := session.New(myLog, nil, nil, -1)

	sub, err := bus.Subscribe()
	require.NoError(t, err)
	lc := lifecycle.New()
	myDeploymentManager := &deploymentManager{
		bus:     bus,
		session: mySession,
		client:  client,
		lease:   lid,
		mgroup:  group,
		log:     myLog,
		lc:      lc,
	}
	monitor := newDeploymentMonitor(myDeploymentManager)
	require.NotNil(t, monitor)

	ev := <-sub.Events()
	result := ev.(event.ClusterDeployment)
	require.Equal(t, lid, result.LeaseID)
	require.Equal(t, event.ClusterDeploymentPending, result.Status)

	monitor.lc.Shutdown(nil)
}

func TestMonitorSendsClusterDeploymentDeployed(t *testing.T) {
	const serviceName = "test"
	myLog := testutil.Logger(t)
	bus := pubsub.NewBus()
	lid := testutil.LeaseID(t)

	group := &manifest.Group{}
	group.Services = make([]manifest.Service, 1)
	group.Services[0].Name = serviceName
	group.Services[0].Expose = make([]manifest.ServiceExpose, 1)
	group.Services[0].Expose[0].ExternalPort = 2000
	group.Services[0].Expose[0].Proto = manifest.TCP
	group.Services[0].Expose[0].Port = 40000
	group.Services[0].Count = 3
	client := &mocks.Client{}

	statusResult := make(map[string]*ctypes.ServiceStatus)
	statusResult[serviceName] = &ctypes.ServiceStatus{
		Name:               serviceName,
		Available:          3,
		Total:              3,
		URIs:               nil,
		ObservedGeneration: 0,
		Replicas:           0,
		UpdatedReplicas:    0,
		ReadyReplicas:      0,
		AvailableReplicas:  0,
	}
	client.On("LeaseStatus", mock.Anything, lid).Return(statusResult, nil)
	mySession := session.New(myLog, nil, nil, -1)

	sub, err := bus.Subscribe()
	require.NoError(t, err)
	lc := lifecycle.New()
	myDeploymentManager := &deploymentManager{
		bus:     bus,
		session: mySession,
		client:  client,
		lease:   lid,
		mgroup:  group,
		log:     myLog,
		lc:      lc,
	}
	monitor := newDeploymentMonitor(myDeploymentManager)
	require.NotNil(t, monitor)

	ev := <-sub.Events()
	result := ev.(event.ClusterDeployment)
	require.Equal(t, lid, result.LeaseID)
	require.Equal(t, event.ClusterDeploymentDeployed, result.Status)

	monitor.lc.Shutdown(nil)
}
