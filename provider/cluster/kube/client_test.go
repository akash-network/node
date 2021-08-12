package kube

// import (
// 	"context"
// 	"errors"
// 	"testing"
//
// 	sdk "github.com/cosmos/cosmos-sdk/types"
// 	"github.com/stretchr/testify/assert"
//
// 	"github.com/ovrclk/akash/manifest"
// 	"github.com/ovrclk/akash/provider/cluster/kube/builder"
// 	"github.com/ovrclk/akash/sdl"
// 	"github.com/ovrclk/akash/types"
// 	mtypes "github.com/ovrclk/akash/x/market/types"
//
// 	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
// 	"k8s.io/apimachinery/pkg/runtime/schema"
//
// 	"github.com/stretchr/testify/mock"
// 	"github.com/stretchr/testify/require"
// 	appsv1 "k8s.io/api/apps/v1"
// 	v1 "k8s.io/api/core/v1"
// 	netv1 "k8s.io/api/networking/v1"
// 	"k8s.io/apimachinery/pkg/api/resource"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/client-go/kubernetes"
//
// 	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
// 	akashclient "github.com/ovrclk/akash/pkg/client/clientset/versioned"
// 	akashclient_fake "github.com/ovrclk/akash/pkg/client/clientset/versioned/fake"
// 	"github.com/ovrclk/akash/testutil"
// 	kubernetes_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock"
// 	appsv1_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/apps/v1"
// 	corev1_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/core/v1"
// 	netv1_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/networking/v1"
//
// 	"github.com/tendermint/tendermint/crypto/ed25519"
// )
//
// func clientForTest(t *testing.T, kc kubernetes.Interface, ac akashclient.Interface) Client {
// 	myLog := testutil.Logger(t)
// 	result := &client{
// 		kc:  kc,
// 		ac:  ac,
// 		log: myLog.With("mode", "test-kube-provider-client"),
// 	}
//
// 	return result
// }
//
// const (
// 	randDSeq uint64 = 1
// 	randGSeq uint32 = 2
// 	randOSeq uint32 = 3
// )
//
// func TestDeploy(t *testing.T) {
// 	t.Skip()
// 	ctx := context.Background()
//
// 	owner := ed25519.GenPrivKey().PubKey().Address()
// 	provider := ed25519.GenPrivKey().PubKey().Address()
//
// 	leaseID := mtypes.LeaseID{
// 		Owner:    sdk.AccAddress(owner).String(),
// 		DSeq:     randDSeq,
// 		GSeq:     randGSeq,
// 		OSeq:     randOSeq,
// 		Provider: sdk.AccAddress(provider).String(),
// 	}
//
// 	sdl, err := sdl.ReadFile("../../../_run/kube/deployment.yaml")
// 	require.NoError(t, err)
//
// 	mani, err := sdl.Manifest()
// 	require.NoError(t, err)
//
// 	log := testutil.Logger(t)
// 	client, err := NewClient(log, "lease", builder.NewDefaultSettings())
// 	assert.NoError(t, err)
//
// 	err = client.Deploy(ctx, leaseID, &mani.GetGroups()[0])
// 	assert.NoError(t, err)
// }
//
// func TestNewClientWithBogusIngressDomain(t *testing.T) {
// 	settings := builder.Settings{
// 		DeploymentIngressStaticHosts: true,
// 		DeploymentIngressDomain:      "*.foo.bar.com",
// 	}
// 	client, err := NewClient(testutil.Logger(t), "aNamespace0", settings)
// 	require.Error(t, err)
// 	require.ErrorIs(t, err, builder.ErrSettingsValidation)
// 	require.Nil(t, client)
//
// 	settings = builder.Settings{
// 		DeploymentIngressStaticHosts: true,
// 		DeploymentIngressDomain:      "foo.bar.com-",
// 	}
// 	client, err = NewClient(testutil.Logger(t), "aNamespace1", settings)
// 	require.Error(t, err)
// 	require.ErrorIs(t, err, builder.ErrSettingsValidation)
// 	require.Nil(t, client)
//
// 	settings = builder.Settings{
// 		DeploymentIngressStaticHosts: true,
// 		DeploymentIngressDomain:      "foo.ba!!!r.com",
// 	}
// 	client, err = NewClient(testutil.Logger(t), "aNamespace2", settings)
// 	require.Error(t, err)
// 	require.ErrorIs(t, err, builder.ErrSettingsValidation)
// 	require.Nil(t, client)
// }
//
// func TestNewClientWithEmptyIngressDomain(t *testing.T) {
// 	settings := builder.Settings{
// 		DeploymentIngressStaticHosts: true,
// 		DeploymentIngressDomain:      "",
// 	}
// 	client, err := NewClient(testutil.Logger(t), "aNamespace3", settings)
// 	require.Error(t, err)
// 	require.ErrorIs(t, err, builder.ErrSettingsValidation)
// 	require.Nil(t, client)
// }
//
// func TestLeaseStatusWithNoDeployments(t *testing.T) {
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(nil, nil)
//
// 	clientInterface := clientForTest(t, kmock, nil)
//
// 	status, err := clientInterface.LeaseStatus(context.Background(), lid)
// 	require.Equal(t, ErrNoDeploymentForLease, err)
// 	require.Nil(t, status)
// }
//
// func TestLeaseStatusWithNoIngressNoService(t *testing.T) {
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	deploymentItems := make([]appsv1.Deployment, 1)
// 	deploymentItems[0].Name = "A"
// 	deploymentItems[0].Status.AvailableReplicas = 10
// 	deploymentItems[0].Status.Replicas = 10
// 	deploymentList := &appsv1.DeploymentList{ // This is concrete so a mock is not used here
// 		TypeMeta: metav1.TypeMeta{},
// 		ListMeta: metav1.ListMeta{},
// 		Items:    deploymentItems,
// 	}
// 	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(deploymentList, nil)
//
// 	netv1Mock := &netv1_mocks.NetworkingV1Interface{}
// 	kmock.On("NetworkingV1").Return(netv1Mock)
// 	ingressesMock := &netv1_mocks.IngressInterface{}
// 	ingressList := &netv1.IngressList{}
//
// 	ingressesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(ingressList, nil)
// 	netv1Mock.On("Ingresses", builder.LidNS(lid)).Return(ingressesMock)
//
// 	servicesMock := &corev1_mocks.ServiceInterface{}
// 	coreV1Mock.On("Services", builder.LidNS(lid)).Return(servicesMock)
//
// 	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
// 	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)
//
// 	clientInterface := clientForTest(t, kmock, nil)
//
// 	status, err := clientInterface.LeaseStatus(context.Background(), lid)
// 	require.Equal(t, ErrNoGlobalServicesForLease, err)
// 	require.Nil(t, status)
// }
//
// func TestLeaseStatusWithIngressOnly(t *testing.T) {
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	deploymentItems := make([]appsv1.Deployment, 2)
// 	deploymentItems[0].Name = "myingress"
// 	deploymentItems[0].Status.AvailableReplicas = 10
// 	deploymentItems[0].Status.Replicas = 10
// 	deploymentItems[1].Name = "noingress"
// 	deploymentItems[1].Status.AvailableReplicas = 1
// 	deploymentItems[1].Status.Replicas = 1
//
// 	deploymentList := &appsv1.DeploymentList{ // This is concrete so a mock is not used here
// 		TypeMeta: metav1.TypeMeta{},
// 		ListMeta: metav1.ListMeta{},
// 		Items:    deploymentItems,
// 	}
//
// 	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(deploymentList, nil)
//
// 	netv1Mock := &netv1_mocks.NetworkingV1Interface{}
// 	kmock.On("NetworkingV1").Return(netv1Mock)
// 	ingressesMock := &netv1_mocks.IngressInterface{}
// 	ingressList := &netv1.IngressList{}
// 	ingressList.Items = make([]netv1.Ingress, 1)
// 	rules := make([]netv1.IngressRule, 1)
// 	rules[0] = netv1.IngressRule{
// 		Host: "mytesthost.dev",
// 	}
// 	ingressList.Items[0] = netv1.Ingress{
//
// 		ObjectMeta: metav1.ObjectMeta{Name: "myingress"},
//
// 		Spec: netv1.IngressSpec{
// 			Rules: rules,
// 		},
// 	}
//
// 	ingressesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(ingressList, nil)
// 	netv1Mock.On("Ingresses", builder.LidNS(lid)).Return(ingressesMock)
//
// 	servicesMock := &corev1_mocks.ServiceInterface{}
// 	coreV1Mock.On("Services", builder.LidNS(lid)).Return(servicesMock)
//
// 	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
// 	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)
//
// 	clientInterface := clientForTest(t, kmock, nil)
//
// 	status, err := clientInterface.LeaseStatus(context.Background(), lid)
// 	require.NoError(t, err)
// 	require.NotNil(t, status)
//
// 	require.Len(t, status.ForwardedPorts, 0)
// 	require.Len(t, status.Services, 2)
// 	services := status.Services
//
// 	myIngressService, found := services["myingress"]
// 	require.True(t, found)
//
// 	require.Equal(t, myIngressService.Name, "myingress")
// 	require.Len(t, myIngressService.URIs, 1)
// 	require.Equal(t, myIngressService.URIs[0], "mytesthost.dev")
//
// 	noIngressService, found := services["noingress"]
// 	require.True(t, found)
//
// 	require.Equal(t, noIngressService.Name, "noingress")
// 	require.Len(t, noIngressService.URIs, 0)
// }
//
// func TestLeaseStatusWithForwardedPortOnly(t *testing.T) {
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	const serviceName = "myservice"
// 	deploymentItems := make([]appsv1.Deployment, 2)
// 	deploymentItems[0].Name = serviceName
// 	deploymentItems[0].Status.AvailableReplicas = 10
// 	deploymentItems[0].Status.Replicas = 10
// 	deploymentItems[1].Name = "noservice"
// 	deploymentItems[1].Status.AvailableReplicas = 1
// 	deploymentItems[1].Status.Replicas = 1
//
// 	deploymentList := &appsv1.DeploymentList{ // This is concrete so a mock is not used here
// 		TypeMeta: metav1.TypeMeta{},
// 		ListMeta: metav1.ListMeta{},
// 		Items:    deploymentItems,
// 	}
//
// 	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(deploymentList, nil)
//
// 	netv1Mock := &netv1_mocks.NetworkingV1Interface{}
// 	kmock.On("NetworkingV1").Return(netv1Mock)
// 	ingressesMock := &netv1_mocks.IngressInterface{}
// 	ingressList := &netv1.IngressList{}
//
// 	ingressesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(ingressList, nil)
// 	netv1Mock.On("Ingresses", builder.LidNS(lid)).Return(ingressesMock)
//
// 	servicesMock := &corev1_mocks.ServiceInterface{}
// 	coreV1Mock.On("Services", builder.LidNS(lid)).Return(servicesMock)
//
// 	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
// 	servicesList.Items = make([]v1.Service, 1)
//
// 	servicesList.Items[0].Name = serviceName + builder.SuffixForNodePortServiceName
//
// 	servicesList.Items[0].Spec.Type = v1.ServiceTypeNodePort
// 	servicesList.Items[0].Spec.Ports = make([]v1.ServicePort, 1)
// 	const expectedExternalPort = 13211
// 	servicesList.Items[0].Spec.Ports[0].NodePort = expectedExternalPort
// 	servicesList.Items[0].Spec.Ports[0].Protocol = v1.ProtocolTCP
// 	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)
//
// 	clientInterface := clientForTest(t, kmock, nil)
//
// 	status, err := clientInterface.LeaseStatus(context.Background(), lid)
// 	require.NoError(t, err)
// 	require.NotNil(t, status)
//
// 	require.Len(t, status.Services, 2)
// 	for _, service := range status.Services {
// 		require.Len(t, service.URIs, 0) // No ingresses, so there should be no URIs
// 	}
// 	require.Len(t, status.ForwardedPorts, 1)
//
// 	ports := status.ForwardedPorts[serviceName]
// 	require.Len(t, ports, 1)
// 	require.Equal(t, int(ports[0].ExternalPort), expectedExternalPort)
// }
//
// func TestServiceStatusNoLease(t *testing.T) {
// 	const serviceName = "foobar"
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	testErr := kubeErrors.NewNotFound(schema.GroupResource{}, "bob")
// 	require.True(t, kubeErrors.IsNotFound(testErr))
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, testErr)
//
// 	clientInterface := clientForTest(t, kmock, nil)
//
// 	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
// 	require.ErrorIs(t, err, ErrLeaseNotFound)
// 	require.Nil(t, status)
// }
//
// func TestServiceStatusNoDeployment(t *testing.T) {
// 	const serviceName = "foobar"
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
// 	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(nil, nil)
//
// 	akashMock := akashclient_fake.NewSimpleClientset()
//
// 	clientInterface := clientForTest(t, kmock, akashMock)
//
// 	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
// 	require.ErrorIs(t, err, ErrNoDeploymentForLease)
// 	require.Nil(t, status)
// }
//
// func TestServiceStatusNoServiceWithName(t *testing.T) {
// 	const serviceName = "foobar"
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	deployment := appsv1.Deployment{}
// 	deployment.Name = "aname0"
// 	deployment.Status.AvailableReplicas = 10
// 	deployment.Status.Replicas = 10
//
// 	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)
//
// 	mg := &manifest.Group{
// 		Name:     "somename",
// 		Services: nil,
// 	}
//
// 	m, err := crd.NewManifest(builder.LidNS(lid), lid, mg)
// 	require.NoError(t, err)
// 	akashMock := akashclient_fake.NewSimpleClientset(m)
//
// 	clientInterface := clientForTest(t, kmock, akashMock)
//
// 	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
// 	require.ErrorIs(t, err, ErrNoServiceForLease)
// 	require.Nil(t, status)
// }
//
// func TestServiceStatusNoCRDManifest(t *testing.T) {
// 	const serviceName = "foobar"
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	deployment := appsv1.Deployment{}
// 	deployment.Name = "aname1"
// 	deployment.Status.AvailableReplicas = 10
// 	deployment.Status.Replicas = 10
//
// 	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)
//
// 	mg := &manifest.Group{
// 		Name:     "somename",
// 		Services: nil,
// 	}
//
// 	m, err := crd.NewManifest(builder.LidNS(lid)+"a", lid, mg)
// 	require.NoError(t, err)
// 	akashMock := akashclient_fake.NewSimpleClientset(m)
//
// 	clientInterface := clientForTest(t, kmock, akashMock)
//
// 	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
// 	require.Error(t, err)
// 	require.Regexp(t, `^manifests.akash.network ".+" not found$`, err)
// 	require.Nil(t, status)
// }
//
// func TestServiceStatusWithIngress(t *testing.T) {
// 	const serviceName = "foobar"
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	deployment := appsv1.Deployment{}
// 	deployment.Name = "aname2"
// 	deployment.Status.AvailableReplicas = 10
// 	deployment.Status.Replicas = 10
//
// 	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)
//
// 	services := make([]manifest.Service, 2)
// 	services[0] = manifest.Service{
// 		Name:      "someService",
// 		Image:     "best/image",
// 		Command:   nil,
// 		Args:      nil,
// 		Env:       nil,
// 		Resources: types.ResourceUnits{},
// 		Count:     1,
// 		Expose: []manifest.ServiceExpose{
// 			{
// 				Port:         9000,
// 				ExternalPort: 9000,
// 				Proto:        "TCP",
// 				Service:      "echo",
// 				Global:       false,
// 				Hosts:        nil,
// 			},
// 		},
// 	}
// 	services[1] = manifest.Service{
// 		Name:      serviceName,
// 		Image:     "best/image",
// 		Command:   nil,
// 		Args:      nil,
// 		Env:       nil,
// 		Resources: types.ResourceUnits{},
// 		Count:     1,
// 		Expose: []manifest.ServiceExpose{
// 			{
// 				Port:         9000,
// 				ExternalPort: 80,
// 				Proto:        "TCP",
// 				Service:      "echo",
// 				Global:       true,
// 				Hosts:        []string{"atest.localhost"},
// 			},
// 		},
// 	}
// 	mg := &manifest.Group{
// 		Name:     "my-awesome-group",
// 		Services: services,
// 	}
//
// 	m, err := crd.NewManifest(builder.LidNS(lid), lid, mg)
// 	require.NoError(t, err)
// 	akashMock := akashclient_fake.NewSimpleClientset(m)
//
// 	netmock := &netv1_mocks.NetworkingV1Interface{}
// 	kmock.On("NetworkingV1").Return(netmock)
//
// 	ingressMock := &netv1_mocks.IngressInterface{}
// 	netmock.On("Ingresses", builder.LidNS(lid)).Return(ingressMock)
//
// 	ingress := &netv1.Ingress{
// 		TypeMeta:   metav1.TypeMeta{},
// 		ObjectMeta: metav1.ObjectMeta{},
// 		Spec: netv1.IngressSpec{
// 			IngressClassName: nil,
// 			DefaultBackend:   nil,
// 			TLS:              nil,
// 			Rules: []netv1.IngressRule{
// 				{
// 					Host:             "abcd.com",
// 					IngressRuleValue: netv1.IngressRuleValue{},
// 				},
// 			},
// 		},
// 		Status: netv1.IngressStatus{},
// 	}
// 	ingressMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(ingress, nil)
//
// 	clientInterface := clientForTest(t, kmock, akashMock)
//
// 	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
// 	require.NoError(t, err)
// 	require.NotNil(t, status)
//
// 	require.Equal(t, status.URIs, []string{"abcd.com"})
// }
//
// var errNoSuchIngress = errors.New("no such ingress")
//
// func TestServiceStatusWithIngressError(t *testing.T) {
// 	const serviceName = "foobar"
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	deployment := appsv1.Deployment{}
// 	deployment.Name = "aname4"
// 	deployment.Status.AvailableReplicas = 10
// 	deployment.Status.Replicas = 10
//
// 	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)
//
// 	services := make([]manifest.Service, 2)
// 	services[0] = manifest.Service{
// 		Name:      "someService",
// 		Image:     "best/image",
// 		Command:   nil,
// 		Args:      nil,
// 		Env:       nil,
// 		Resources: types.ResourceUnits{},
// 		Count:     1,
// 		Expose: []manifest.ServiceExpose{
// 			{
// 				Port:         9000,
// 				ExternalPort: 9000,
// 				Proto:        "TCP",
// 				Service:      "echo",
// 				Global:       false,
// 				Hosts:        nil,
// 			},
// 		},
// 	}
// 	services[1] = manifest.Service{
// 		Name:      serviceName,
// 		Image:     "best/image",
// 		Command:   nil,
// 		Args:      nil,
// 		Env:       nil,
// 		Resources: types.ResourceUnits{},
// 		Count:     1,
// 		Expose: []manifest.ServiceExpose{
// 			{
// 				Port:         9000,
// 				ExternalPort: 80,
// 				Proto:        "TCP",
// 				Service:      "echo",
// 				Global:       true,
// 				Hosts:        []string{"atest.localhost"},
// 			},
// 		},
// 	}
// 	mg := &manifest.Group{
// 		Name:     "my-awesome-group",
// 		Services: services,
// 	}
//
// 	m, err := crd.NewManifest(builder.LidNS(lid), lid, mg)
// 	require.NoError(t, err)
// 	akashMock := akashclient_fake.NewSimpleClientset(m)
//
// 	netmock := &netv1_mocks.NetworkingV1Interface{}
// 	kmock.On("NetworkingV1").Return(netmock)
//
// 	ingressMock := &netv1_mocks.IngressInterface{}
// 	netmock.On("Ingresses", builder.LidNS(lid)).Return(ingressMock)
//
// 	ingressMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(nil, errNoSuchIngress)
// 	clientInterface := clientForTest(t, kmock, akashMock)
//
// 	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
// 	require.ErrorIs(t, err, errNoSuchIngress)
// 	require.Nil(t, status)
// }
//
// func TestServiceStatusWithoutIngress(t *testing.T) {
// 	const serviceName = "foobar"
// 	lid := testutil.LeaseID(t)
//
// 	kmock := &kubernetes_mocks.Interface{}
// 	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
// 	coreV1Mock := &corev1_mocks.CoreV1Interface{}
// 	kmock.On("AppsV1").Return(appsV1Mock)
// 	kmock.On("CoreV1").Return(coreV1Mock)
//
// 	namespaceMock := &corev1_mocks.NamespaceInterface{}
// 	coreV1Mock.On("Namespaces").Return(namespaceMock)
// 	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)
//
// 	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
// 	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
//
// 	deployment := appsv1.Deployment{}
// 	deployment.Name = "aname5"
// 	deployment.Status.AvailableReplicas = 10
// 	deployment.Status.Replicas = 10
//
// 	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)
//
// 	services := make([]manifest.Service, 2)
// 	services[0] = manifest.Service{
// 		Name:      "someService",
// 		Image:     "best/image",
// 		Command:   nil,
// 		Args:      nil,
// 		Env:       nil,
// 		Resources: types.ResourceUnits{},
// 		Count:     1,
// 		Expose: []manifest.ServiceExpose{
// 			{
// 				Port:         9000,
// 				ExternalPort: 9000,
// 				Proto:        "TCP",
// 				Service:      "echo",
// 				Global:       false,
// 				Hosts:        nil,
// 			},
// 		},
// 	}
// 	services[1] = manifest.Service{
// 		Name:      serviceName,
// 		Image:     "best/image",
// 		Command:   nil,
// 		Args:      nil,
// 		Env:       nil,
// 		Resources: types.ResourceUnits{},
// 		Count:     1,
// 		Expose: []manifest.ServiceExpose{
// 			{
// 				Port:         9000,
// 				ExternalPort: 80,
// 				Proto:        "TCP",
// 				Service:      "echo",
// 				Global:       false,
// 				Hosts:        []string{"atest.localhost"},
// 			},
// 		},
// 	}
// 	mg := &manifest.Group{
// 		Name:     "my-awesome-group",
// 		Services: services,
// 	}
//
// 	m, err := crd.NewManifest(builder.LidNS(lid), lid, mg)
// 	require.NoError(t, err)
// 	akashMock := akashclient_fake.NewSimpleClientset(m)
//
// 	clientInterface := clientForTest(t, kmock, akashMock)
//
// 	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
// 	require.NoError(t, err)
// 	require.NotNil(t, status)
// 	require.Len(t, status.URIs, 0)
// }
//
// type inventoryScaffold struct {
// 	kmock             *kubernetes_mocks.Interface
// 	corev1Mock        *corev1_mocks.CoreV1Interface
// 	nodeInterfaceMock *corev1_mocks.NodeInterface
// 	podInterfaceMock  *corev1_mocks.PodInterface
// }
//
// func makeInventoryScaffold() *inventoryScaffold {
// 	s := &inventoryScaffold{
// 		kmock:             &kubernetes_mocks.Interface{},
// 		corev1Mock:        &corev1_mocks.CoreV1Interface{},
// 		nodeInterfaceMock: &corev1_mocks.NodeInterface{},
// 		podInterfaceMock:  &corev1_mocks.PodInterface{},
// 	}
//
// 	s.kmock.On("CoreV1").Return(s.corev1Mock)
// 	s.corev1Mock.On("Nodes").Return(s.nodeInterfaceMock, nil)
// 	s.corev1Mock.On("Pods", "" /* all namespaces */).Return(s.podInterfaceMock, nil)
//
// 	return s
// }
//
// func TestInventoryZero(t *testing.T) {
// 	s := makeInventoryScaffold()
//
// 	nodeList := &v1.NodeList{}
// 	listOptions := metav1.ListOptions{}
// 	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nodeList, nil)
//
// 	podList := &v1.PodList{}
// 	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)
//
// 	clientInterface := clientForTest(t, s.kmock, nil)
// 	inventory, err := clientInterface.Inventory(context.Background())
// 	require.NoError(t, err)
// 	require.NotNil(t, inventory)
//
// 	// The inventory was called and the kubernetes client says there are no nodes & no pods. Inventory
// 	// should be zero
// 	require.Len(t, inventory, 0)
//
// 	podListOptionsInCall := s.podInterfaceMock.Calls[0].Arguments[1].(metav1.ListOptions)
// 	require.Equal(t, "status.phase!=Failed,status.phase!=Succeeded", podListOptionsInCall.FieldSelector)
// }
//
// func TestInventorySingleNodeNoPods(t *testing.T) {
// 	s := makeInventoryScaffold()
//
// 	nodeList := &v1.NodeList{}
// 	nodeList.Items = make([]v1.Node, 1)
//
// 	nodeResourceList := make(v1.ResourceList)
// 	const expectedCPU = 13
// 	cpuQuantity := resource.NewQuantity(expectedCPU, "m")
// 	nodeResourceList[v1.ResourceCPU] = *cpuQuantity
//
// 	const expectedMemory = 14
// 	memoryQuantity := resource.NewQuantity(expectedMemory, "M")
// 	nodeResourceList[v1.ResourceMemory] = *memoryQuantity
//
// 	const expectedStorage = 15
// 	ephemeralStorageQuantity := resource.NewQuantity(expectedStorage, "M")
// 	nodeResourceList[v1.ResourceEphemeralStorage] = *ephemeralStorageQuantity
//
// 	nodeConditions := make([]v1.NodeCondition, 1)
// 	nodeConditions[0] = v1.NodeCondition{
// 		Type:   v1.NodeReady,
// 		Status: v1.ConditionTrue,
// 	}
//
// 	nodeList.Items[0] = v1.Node{
// 		TypeMeta:   metav1.TypeMeta{},
// 		ObjectMeta: metav1.ObjectMeta{},
// 		Spec:       v1.NodeSpec{},
// 		Status: v1.NodeStatus{
// 			Allocatable: nodeResourceList,
// 			Conditions:  nodeConditions,
// 		},
// 	}
//
// 	listOptions := metav1.ListOptions{}
// 	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nodeList, nil)
//
// 	podList := &v1.PodList{}
// 	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)
//
// 	clientInterface := clientForTest(t, s.kmock, nil)
// 	inventory, err := clientInterface.Inventory(context.Background())
// 	require.NoError(t, err)
// 	require.NotNil(t, inventory)
//
// 	require.Len(t, inventory, 1)
//
// 	node := inventory[0]
// 	availableResources := node.Available()
// 	// Multiply expected value by 1000 since millicpu is used
// 	require.Equal(t, uint64(expectedCPU*1000), availableResources.CPU.Units.Value())
// 	require.Equal(t, uint64(expectedMemory), availableResources.Memory.Quantity.Value())
// 	require.Equal(t, uint64(expectedStorage), availableResources.Storage.Quantity.Value())
// }
//
// func TestInventorySingleNodeWithPods(t *testing.T) {
// 	s := makeInventoryScaffold()
//
// 	nodeList := &v1.NodeList{}
// 	nodeList.Items = make([]v1.Node, 1)
//
// 	nodeResourceList := make(v1.ResourceList)
// 	const expectedCPU = 13
// 	cpuQuantity := resource.NewQuantity(expectedCPU, "m")
// 	nodeResourceList[v1.ResourceCPU] = *cpuQuantity
//
// 	const expectedMemory = 2048
// 	memoryQuantity := resource.NewQuantity(expectedMemory, "M")
// 	nodeResourceList[v1.ResourceMemory] = *memoryQuantity
//
// 	const expectedStorage = 4096
// 	ephemeralStorageQuantity := resource.NewQuantity(expectedStorage, "M")
// 	nodeResourceList[v1.ResourceEphemeralStorage] = *ephemeralStorageQuantity
//
// 	nodeConditions := make([]v1.NodeCondition, 1)
// 	nodeConditions[0] = v1.NodeCondition{
// 		Type:   v1.NodeReady,
// 		Status: v1.ConditionTrue,
// 	}
//
// 	nodeList.Items[0] = v1.Node{
// 		TypeMeta:   metav1.TypeMeta{},
// 		ObjectMeta: metav1.ObjectMeta{},
// 		Spec:       v1.NodeSpec{},
// 		Status: v1.NodeStatus{
// 			Allocatable: nodeResourceList,
// 			Conditions:  nodeConditions,
// 		},
// 	}
//
// 	listOptions := metav1.ListOptions{}
// 	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nodeList, nil)
//
// 	const cpuPerContainer = 1
// 	const memoryPerContainer = 3
// 	const storagePerContainer = 17
// 	// Define two pods
// 	pods := make([]v1.Pod, 2)
// 	// First pod has 1 container
// 	podContainers := make([]v1.Container, 1)
// 	containerRequests := make(v1.ResourceList)
// 	cpuQuantity.SetMilli(cpuPerContainer)
// 	containerRequests[v1.ResourceCPU] = *cpuQuantity
//
// 	memoryQuantity = resource.NewQuantity(memoryPerContainer, "M")
// 	containerRequests[v1.ResourceMemory] = *memoryQuantity
//
// 	ephemeralStorageQuantity = resource.NewQuantity(storagePerContainer, "M")
// 	containerRequests[v1.ResourceEphemeralStorage] = *ephemeralStorageQuantity
//
// 	podContainers[0] = v1.Container{
// 		Resources: v1.ResourceRequirements{
// 			Limits:   nil,
// 			Requests: containerRequests,
// 		},
// 	}
// 	pods[0] = v1.Pod{
// 		TypeMeta:   metav1.TypeMeta{},
// 		ObjectMeta: metav1.ObjectMeta{},
// 		Spec: v1.PodSpec{
// 			Containers: podContainers,
// 		},
// 		Status: v1.PodStatus{},
// 	}
//
// 	// Define 2nd pod with multiple containers
// 	podContainers = make([]v1.Container, 2)
// 	for i := range podContainers {
// 		containerRequests := make(v1.ResourceList)
// 		cpuQuantity.SetMilli(cpuPerContainer)
// 		containerRequests[v1.ResourceCPU] = *cpuQuantity
//
// 		memoryQuantity = resource.NewQuantity(memoryPerContainer, "M")
// 		containerRequests[v1.ResourceMemory] = *memoryQuantity
//
// 		ephemeralStorageQuantity = resource.NewQuantity(storagePerContainer, "M")
// 		containerRequests[v1.ResourceEphemeralStorage] = *ephemeralStorageQuantity
//
// 		// Container limits are enforced by kubernetes as absolute limits, but not
// 		// used when considering inventory since overcommit is possible in a kubernetes cluster
// 		// Set limits to any value larger than requests in this test since it should not change
// 		// the value returned  by the code
// 		containerLimits := make(v1.ResourceList)
//
// 		for k, v := range containerRequests {
// 			replacementV := resource.NewQuantity(0, "")
// 			replacementV.Set(v.Value() * int64(testutil.RandRangeInt(2, 100)))
// 			containerLimits[k] = *replacementV
// 		}
//
// 		podContainers[i] = v1.Container{
// 			Resources: v1.ResourceRequirements{
// 				Limits:   containerLimits,
// 				Requests: containerRequests,
// 			},
// 		}
// 	}
// 	pods[1] = v1.Pod{
// 		TypeMeta:   metav1.TypeMeta{},
// 		ObjectMeta: metav1.ObjectMeta{},
// 		Spec: v1.PodSpec{
// 			Containers: podContainers,
// 		},
// 		Status: v1.PodStatus{},
// 	}
//
// 	podList := &v1.PodList{
// 		Items: pods,
// 	}
//
// 	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)
//
// 	clientInterface := clientForTest(t, s.kmock, nil)
// 	inventory, err := clientInterface.Inventory(context.Background())
// 	require.NoError(t, err)
// 	require.NotNil(t, inventory)
//
// 	require.Len(t, inventory, 1)
//
// 	node := inventory[0]
// 	availableResources := node.Available()
// 	// Multiply expected value by 1000 since millicpu is used
// 	require.Equal(t, uint64(expectedCPU*1000)-3*cpuPerContainer, availableResources.CPU.Units.Value())
// 	require.Equal(t, uint64(expectedMemory)-3*memoryPerContainer, availableResources.Memory.Quantity.Value())
// 	require.Equal(t, uint64(expectedStorage)-3*storagePerContainer, availableResources.Storage.Quantity.Value())
// }
//
// var errForTest = errors.New("error in test")
//
// func TestInventoryWithNodeError(t *testing.T) {
// 	s := makeInventoryScaffold()
//
// 	listOptions := metav1.ListOptions{}
// 	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nil, errForTest)
//
// 	clientInterface := clientForTest(t, s.kmock, nil)
// 	inventory, err := clientInterface.Inventory(context.Background())
// 	require.Error(t, err)
// 	require.True(t, errors.Is(err, errForTest))
// 	require.Nil(t, inventory)
// }
//
// func TestInventoryWithPodsError(t *testing.T) {
// 	s := makeInventoryScaffold()
//
// 	listOptions := metav1.ListOptions{}
// 	nodeList := &v1.NodeList{}
// 	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nodeList, nil)
// 	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nil, errForTest)
//
// 	clientInterface := clientForTest(t, s.kmock, nil)
// 	inventory, err := clientInterface.Inventory(context.Background())
// 	require.Error(t, err)
// 	require.True(t, errors.Is(err, errForTest))
// 	require.Nil(t, inventory)
// }
