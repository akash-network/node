package kube

import (
	"context"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/testutil"
	kubernetes_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock"
	appsv1_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/apps/v1"
	corev1_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/core/v1"
	netv1_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/networking/v1"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"testing"
)

func clientForTest(t *testing.T, kc kubernetes.Interface) Client {
	myLog := testutil.Logger(t)
	result := &client{
		kc:  kc,
		log: myLog.With("mode", "test-kube-provider-client"),
	}

	return result
}

func TestShouldExpose(t *testing.T) {
	// Should not create ingress for something on port 81
	require.False(t, shouldExpose(&manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.TCP,
		Port:   81,
	}))

	// Should create ingress for something on port 80
	require.True(t, shouldExpose(&manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.TCP,
		Port:   80,
	}))

	// Should not create ingress for something on port 80 that is not Global
	require.False(t, shouldExpose(&manifest.ServiceExpose{
		Global: false,
		Proto:  manifest.TCP,
		Port:   80,
	}))

	// Should not create ingress for something on port 80 that is UDP
	require.False(t, shouldExpose(&manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.UDP,
		Port:   80,
	}))
}

func TestLeaseStatusWithNoDeployments(t *testing.T) {
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", lidNS(lid)).Return(deploymentsMock)

	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(nil, nil)

	clientInterface := clientForTest(t, kmock)

	status, err := clientInterface.LeaseStatus(context.Background(), lid)
	require.Equal(t, ErrNoDeploymentForLease, err)
	require.Nil(t, status)
}

func TestLeaseStatusWithNoIngressNoService(t *testing.T) {
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", lidNS(lid)).Return(deploymentsMock)

	deploymentItems := make([]appsv1.Deployment, 1)
	deploymentItems[0].Name = "A"
	deploymentItems[0].Status.AvailableReplicas = 10
	deploymentItems[0].Status.Replicas = 10
	deploymentList := &appsv1.DeploymentList{ // This is concrete so a mock is not used here
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    deploymentItems,
	}
	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(deploymentList, nil)

	netv1Mock := &netv1_mocks.NetworkingV1Interface{}
	kmock.On("NetworkingV1").Return(netv1Mock)
	ingressesMock := &netv1_mocks.IngressInterface{}
	ingressList := &netv1.IngressList{}
	ingressesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(ingressList, nil)
	netv1Mock.On("Ingresses", lidNS(lid)).Return(ingressesMock)

	corev1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("CoreV1").Return(corev1Mock)

	servicesMock := &corev1_mocks.ServiceInterface{}
	corev1Mock.On("Services", lidNS(lid)).Return(servicesMock)

	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)

	clientInterface := clientForTest(t, kmock)

	status, err := clientInterface.LeaseStatus(context.Background(), lid)
	require.Equal(t, ErrNoGlobalServicesForLease, err)
	require.Nil(t, status)
}

func TestLeaseStatusWithIngressOnly(t *testing.T) {
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", lidNS(lid)).Return(deploymentsMock)

	deploymentItems := make([]appsv1.Deployment, 2)
	deploymentItems[0].Name = "myingress"
	deploymentItems[0].Status.AvailableReplicas = 10
	deploymentItems[0].Status.Replicas = 10
	deploymentItems[1].Name = "noingress"
	deploymentItems[1].Status.AvailableReplicas = 1
	deploymentItems[1].Status.Replicas = 1

	deploymentList := &appsv1.DeploymentList{ // This is concrete so a mock is not used here
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    deploymentItems,
	}

	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(deploymentList, nil)

	netv1Mock := &netv1_mocks.NetworkingV1Interface{}
	kmock.On("NetworkingV1").Return(netv1Mock)
	ingressesMock := &netv1_mocks.IngressInterface{}
	ingressList := &netv1.IngressList{}
	ingressList.Items = make([]netv1.Ingress, 1)
	rules := make([]netv1.IngressRule, 1)
	rules[0] = netv1.IngressRule{
		Host: "mytesthost.dev",
	}
	ingressList.Items[0] = netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "myingress"},
		Spec: netv1.IngressSpec{
			Rules: rules,
		},
	}

	ingressesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(ingressList, nil)
	netv1Mock.On("Ingresses", lidNS(lid)).Return(ingressesMock)

	corev1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("CoreV1").Return(corev1Mock)

	servicesMock := &corev1_mocks.ServiceInterface{}
	corev1Mock.On("Services", lidNS(lid)).Return(servicesMock)

	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)

	clientInterface := clientForTest(t, kmock)

	status, err := clientInterface.LeaseStatus(context.Background(), lid)
	require.NoError(t, err)
	require.NotNil(t, status)

	require.Len(t, status.ForwardedPorts, 0)
	require.Len(t, status.Services, 2)
	services := status.Services

	myIngressService, found := services["myingress"]
	require.True(t, found)

	require.Equal(t, myIngressService.Name, "myingress")
	require.Len(t, myIngressService.URIs, 1)
	require.Equal(t, myIngressService.URIs[0], "mytesthost.dev")

	noIngressService, found := services["noingress"]
	require.True(t, found)

	require.Equal(t, noIngressService.Name, "noingress")
	require.Len(t, noIngressService.URIs, 0)
}

func TestLeaseStatusWithForwardedPortOnly(t *testing.T) {
	lid := testutil.LeaseID(t)
	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", lidNS(lid)).Return(deploymentsMock)

	deploymentItems := make([]appsv1.Deployment, 2)
	deploymentItems[0].Name = "myservice"
	deploymentItems[0].Status.AvailableReplicas = 10
	deploymentItems[0].Status.Replicas = 10
	deploymentItems[1].Name = "noservice"
	deploymentItems[1].Status.AvailableReplicas = 1
	deploymentItems[1].Status.Replicas = 1

	deploymentList := &appsv1.DeploymentList{ // This is concrete so a mock is not used here
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    deploymentItems,
	}

	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(deploymentList, nil)

	netv1Mock := &netv1_mocks.NetworkingV1Interface{}
	kmock.On("NetworkingV1").Return(netv1Mock)
	ingressesMock := &netv1_mocks.IngressInterface{}
	ingressList := &netv1.IngressList{}

	ingressesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(ingressList, nil)
	netv1Mock.On("Ingresses", lidNS(lid)).Return(ingressesMock)

	corev1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("CoreV1").Return(corev1Mock)

	servicesMock := &corev1_mocks.ServiceInterface{}
	corev1Mock.On("Services", lidNS(lid)).Return(servicesMock)

	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
	servicesList.Items = make([]v1.Service, 1)
	servicesList.Items[0].Name = "myservice" + suffixForNodePortServiceName
	servicesList.Items[0].Spec.Type = v1.ServiceTypeNodePort
	servicesList.Items[0].Spec.Ports = make([]v1.ServicePort, 1)
	const expectedExternalPort = 13211
	servicesList.Items[0].Spec.Ports[0].NodePort = expectedExternalPort
	servicesList.Items[0].Spec.Ports[0].Protocol = v1.ProtocolTCP
	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)

	clientInterface := clientForTest(t, kmock)

	status, err := clientInterface.LeaseStatus(context.Background(), lid)
	require.NoError(t, err)
	require.NotNil(t, status)

	require.Len(t, status.Services, 2)
	for _, service := range status.Services {
		require.Len(t, service.URIs, 0) // No ingresses, so there should be no URIs
	}
	require.Len(t, status.ForwardedPorts, 1)

	ports := status.ForwardedPorts["myservice"]
	require.Len(t, ports, 1)
	require.Equal(t, int(ports[0].ExternalPort), expectedExternalPort)

}
