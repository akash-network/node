package builder

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	manitypes "github.com/ovrclk/akash/manifest/v2beta1"
	"github.com/ovrclk/akash/testutil"
)

const testKubeClientNs = "nstest1111"

func TestLidNsSanity(t *testing.T) {
	log := testutil.Logger(t)
	leaseID := testutil.LeaseID(t)

	settings := NewDefaultSettings()
	g := &manitypes.Group{}

	mb := BuildManifest(log, settings, testKubeClientNs, leaseID, g)
	require.Equal(t, testKubeClientNs, mb.NS())

	m, err := mb.Create()
	require.NoError(t, err)
	require.Equal(t, m.Spec.LeaseID.DSeq, strconv.FormatUint(leaseID.DSeq, 10))

	require.Equal(t, LidNS(leaseID), m.Name)
}

// func TestNetworkPolicies(t *testing.T) {
// 	leaseID := testutil.LeaseID(t)
//
// 	g := &manitypes.Group{}
// 	settings := NewDefaultSettings()
// 	np := BuildNetPol(NewDefaultSettings(), leaseID, g)
//
// 	// disabled
// 	netPolicies, err := np.Create()
// 	assert.NoError(t, err)
// 	assert.Len(t, netPolicies, 0)
//
// 	// enabled
// 	settings.NetworkPoliciesEnabled = true
// 	np = BuildNetPol(settings, leaseID, g)
// 	netPolicies, err = np.Create()
// 	assert.NoError(t, err)
// 	assert.Len(t, netPolicies, 1)
//
// 	pol0 := netPolicies[0]
// 	assert.Equal(t, pol0.Name, "akash-deployment-restrictions")
//
// 	// Change the DSeq ID
// 	np.DSeq = uint64(100)
// 	k := akashNetworkNamespace
// 	ns := LidNS(np.lid)
// 	updatedNetPol, err := np.Update(netPolicies[0])
// 	assert.NoError(t, err)
// 	updatedNS := updatedNetPol.Labels[k]
// 	assert.Equal(t, ns, updatedNS)
// }

func TestGlobalServiceBuilder(t *testing.T) {
	myLog := testutil.Logger(t)
	group := &manitypes.Group{}
	service := &manitypes.Service{
		Name: "myservice",
	}
	mySettings := NewDefaultSettings()
	lid := testutil.LeaseID(t)
	serviceBuilder := BuildService(myLog, mySettings, lid, group, service, true)
	require.NotNil(t, serviceBuilder)
	// Should have name ending with suffix
	require.Equal(t, "myservice-np", serviceBuilder.Name())
	// Should not have any work to do
	require.False(t, serviceBuilder.Any())
}

func TestLocalServiceBuilder(t *testing.T) {
	myLog := testutil.Logger(t)
	group := &manitypes.Group{}
	service := &manitypes.Service{
		Name: "myservice",
	}
	mySettings := NewDefaultSettings()
	lid := testutil.LeaseID(t)
	serviceBuilder := BuildService(myLog, mySettings, lid, group, service, false)
	require.NotNil(t, serviceBuilder)
	// Should have name verbatim
	require.Equal(t, "myservice", serviceBuilder.Name())
	// Should not have any work to do
	require.False(t, serviceBuilder.Any())
}

func TestGlobalServiceBuilderWithoutGlobalServices(t *testing.T) {
	myLog := testutil.Logger(t)
	group := &manitypes.Group{}
	exposesServices := make([]manitypes.ServiceExpose, 1)
	exposesServices[0].Global = false
	service := &manitypes.Service{
		Name:   "myservice",
		Expose: exposesServices,
	}
	mySettings := NewDefaultSettings()
	lid := testutil.LeaseID(t)
	serviceBuilder := BuildService(myLog, mySettings, lid, group, service, true)

	// Should not have any work to do
	require.False(t, serviceBuilder.Any())
}

func TestGlobalServiceBuilderWithGlobalServices(t *testing.T) {
	myLog := testutil.Logger(t)
	group := &manitypes.Group{}
	exposesServices := make([]manitypes.ServiceExpose, 2)
	exposesServices[0] = manitypes.ServiceExpose{
		Global:       true,
		Proto:        "TCP",
		Port:         1000,
		ExternalPort: 1001,
	}
	exposesServices[1] = manitypes.ServiceExpose{
		Global:       false,
		Proto:        "TCP",
		Port:         2000,
		ExternalPort: 2001,
	}
	service := &manitypes.Service{
		Name:   "myservice",
		Expose: exposesServices,
	}
	mySettings := NewDefaultSettings()
	lid := testutil.LeaseID(t)
	serviceBuilder := BuildService(myLog, mySettings, lid, group, service, true)

	// Should have work to do
	require.True(t, serviceBuilder.Any())

	result, err := serviceBuilder.Create()
	require.NoError(t, err)
	require.Equal(t, result.Spec.Type, corev1.ServiceTypeNodePort)
	ports := result.Spec.Ports
	require.Len(t, ports, 1)
	require.Equal(t, ports[0].Port, int32(1001))
	require.Equal(t, ports[0].TargetPort, intstr.FromInt(1000))
	require.Equal(t, ports[0].Name, "0-1001")
}

func TestLocalServiceBuilderWithoutLocalServices(t *testing.T) {
	myLog := testutil.Logger(t)
	group := &manitypes.Group{}
	exposesServices := make([]manitypes.ServiceExpose, 1)
	exposesServices[0].Global = true
	service := &manitypes.Service{
		Name:   "myservice",
		Expose: exposesServices,
	}
	mySettings := NewDefaultSettings()
	lid := testutil.LeaseID(t)
	serviceBuilder := BuildService(myLog, mySettings, lid, group, service, false)

	// Should have work to do
	require.False(t, serviceBuilder.Any())
}

func TestLocalServiceBuilderWithLocalServices(t *testing.T) {
	myLog := testutil.Logger(t)
	group := &manitypes.Group{}
	exposesServices := make([]manitypes.ServiceExpose, 2)
	exposesServices[0] = manitypes.ServiceExpose{
		Global:       true,
		Proto:        "TCP",
		Port:         1000,
		ExternalPort: 1001,
	}
	exposesServices[1] = manitypes.ServiceExpose{
		Global:       false,
		Proto:        "TCP",
		Port:         2000,
		ExternalPort: 2001,
	}
	service := &manitypes.Service{
		Name:   "myservice",
		Expose: exposesServices,
	}
	mySettings := NewDefaultSettings()
	lid := testutil.LeaseID(t)
	serviceBuilder := BuildService(myLog, mySettings, lid, group, service, false)

	// Should have work to do
	require.True(t, serviceBuilder.Any())

	result, err := serviceBuilder.Create()
	require.NoError(t, err)
	require.Equal(t, result.Spec.Type, corev1.ServiceTypeClusterIP)
	ports := result.Spec.Ports
	require.Equal(t, ports[0].Port, int32(2001))
	require.Equal(t, ports[0].TargetPort, intstr.FromInt(2000))
	require.Equal(t, ports[0].Name, "1-2001")
}
