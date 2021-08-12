package kube

import (
	"context"
	"testing"

	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	"github.com/ovrclk/akash/types"
	mtypes "github.com/ovrclk/akash/x/market/types"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	akashclient "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	akashclient_fake "github.com/ovrclk/akash/pkg/client/clientset/versioned/fake"
	"github.com/ovrclk/akash/testutil"
	kubernetes_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock"
	appsv1_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/apps/v1"
	corev1_mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/core/v1"

	akashv1_types "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
)

const testKubeClientNs = "nstest1111"

func clientForTest(t *testing.T, kc kubernetes.Interface, ac akashclient.Interface) Client {
	myLog := testutil.Logger(t)
	result := &client{
		kc:  kc,
		ac:  ac,
		log: myLog.With("mode", "test-kube-provider-client"),
		ns:  testKubeClientNs,
	}

	return result
}

func TestNewClientWithBogusIngressDomain(t *testing.T) {
	settings := builder.Settings{
		DeploymentIngressStaticHosts: true,
		DeploymentIngressDomain:      "*.foo.bar.com",
	}
	ctx := context.WithValue(context.Background(), builder.SettingsKey, settings)

	kmock := &kubernetes_mocks.Interface{}
	client := clientForTest(t, kmock, nil)
	require.NotNil(t, client)

	result, err := client.LeaseStatus(ctx, testutil.LeaseID(t))
	require.Error(t, err)
	require.ErrorIs(t, err, builder.ErrSettingsValidation)
	require.Nil(t, result)

	settings = builder.Settings{
		DeploymentIngressStaticHosts: true,
		DeploymentIngressDomain:      "foo.bar.com-",
	}
	ctx = context.WithValue(context.Background(), builder.SettingsKey, settings)
	result, err = client.LeaseStatus(ctx, testutil.LeaseID(t))
	require.Error(t, err)
	require.ErrorIs(t, err, builder.ErrSettingsValidation)
	require.Nil(t, result)

	settings = builder.Settings{
		DeploymentIngressStaticHosts: true,
		DeploymentIngressDomain:      "foo.ba!!!r.com",
	}
	ctx = context.WithValue(context.Background(), builder.SettingsKey, settings)
	result, err = client.LeaseStatus(ctx, testutil.LeaseID(t))
	require.Error(t, err)
	require.ErrorIs(t, err, builder.ErrSettingsValidation)
	require.Nil(t, result)
}

func TestNewClientWithEmptyIngressDomain(t *testing.T) {
	settings := builder.Settings{
		DeploymentIngressStaticHosts: true,
		DeploymentIngressDomain:      "",
	}

	kmock := &kubernetes_mocks.Interface{}
	client := clientForTest(t, kmock, nil)

	ctx := context.WithValue(context.Background(), builder.SettingsKey, settings)
	result, err := client.LeaseStatus(ctx, testutil.LeaseID(t))
	require.Error(t, err)
	require.ErrorIs(t, err, builder.ErrSettingsValidation)
	require.Nil(t, result)

}

func TestLeaseStatusWithNoDeployments(t *testing.T) {
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

	deploymentsMock.On("List", mock.Anything, metav1.ListOptions{}).Return(nil, nil)

	clientInterface := clientForTest(t, kmock, nil)

	ctx := context.WithValue(context.Background(), builder.SettingsKey, builder.Settings{
		ClusterPublicHostname: "meow.com",
	})
	status, err := clientInterface.LeaseStatus(ctx, lid)
	require.Equal(t, ErrNoDeploymentForLease, err)
	require.Nil(t, status)
}

func TestLeaseStatusWithNoIngressNoService(t *testing.T) {
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	akashMock := akashclient_fake.NewSimpleClientset() // TODO - add objects
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

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

	servicesMock := &corev1_mocks.ServiceInterface{}
	coreV1Mock.On("Services", builder.LidNS(lid)).Return(servicesMock)

	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)

	clientInterface := clientForTest(t, kmock, akashMock)

	ctx := context.WithValue(context.Background(), builder.SettingsKey, builder.Settings{
		ClusterPublicHostname: "meow.com",
	})
	status, err := clientInterface.LeaseStatus(ctx, lid)
	require.NoError(t, err)
	require.NotNil(t, status)

	// TODO - more coverage on the status object
}

func fakeProviderHost(hostname string, leaseID mtypes.LeaseID, serviceName string, externalPort uint32) runtime.Object {
	labels := make(map[string]string)
	builder.AppendLeaseLabels(leaseID, labels)
	return &akashv1_types.ProviderHost{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:                       hostname,
			GenerateName:               "",
			Namespace:                  testKubeClientNs,
			UID:                        "",
			ResourceVersion:            "",
			Generation:                 0,
			CreationTimestamp:          metav1.Time{},
			DeletionTimestamp:          nil,
			DeletionGracePeriodSeconds: nil,
			Labels:                     labels,
			Annotations:                nil,
			OwnerReferences:            nil,
			Finalizers:                 nil,
			ClusterName:                "",
			ManagedFields:              nil,
		},
		Spec: akashv1_types.ProviderHostSpec{
			Owner:        leaseID.Owner,
			Provider:     leaseID.Provider,
			Hostname:     hostname,
			Dseq:         leaseID.DSeq,
			Gseq:         leaseID.GSeq,
			Oseq:         leaseID.OSeq,
			ServiceName:  serviceName,
			ExternalPort: externalPort,
		},
		Status: akashv1_types.ProviderHostStatus{},
	}
}

func TestLeaseStatusWithIngressOnly(t *testing.T) {
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)
	akashMock := akashclient_fake.NewSimpleClientset(fakeProviderHost("mytesthost.dev", lid, "myingress", 1337))

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

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

	servicesMock := &corev1_mocks.ServiceInterface{}
	coreV1Mock.On("Services", builder.LidNS(lid)).Return(servicesMock)

	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)

	clientInterface := clientForTest(t, kmock, akashMock)

	ctx := context.WithValue(context.Background(), builder.SettingsKey, builder.Settings{
		ClusterPublicHostname: "meow.com",
	})

	status, err := clientInterface.LeaseStatus(ctx, lid)
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
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)
	akashMock := akashclient_fake.NewSimpleClientset() // TODO - add objects

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

	const serviceName = "myservice"
	deploymentItems := make([]appsv1.Deployment, 2)
	deploymentItems[0].Name = serviceName
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

	servicesMock := &corev1_mocks.ServiceInterface{}
	coreV1Mock.On("Services", builder.LidNS(lid)).Return(servicesMock)

	servicesList := &v1.ServiceList{} // This is concrete so no mock is used
	servicesList.Items = make([]v1.Service, 1)

	servicesList.Items[0].Name = serviceName + builder.SuffixForNodePortServiceName

	servicesList.Items[0].Spec.Type = v1.ServiceTypeNodePort
	servicesList.Items[0].Spec.Ports = make([]v1.ServicePort, 1)
	const expectedExternalPort = 13211
	servicesList.Items[0].Spec.Ports[0].NodePort = expectedExternalPort
	servicesList.Items[0].Spec.Ports[0].Protocol = v1.ProtocolTCP
	servicesMock.On("List", mock.Anything, metav1.ListOptions{}).Return(servicesList, nil)

	clientInterface := clientForTest(t, kmock, akashMock)

	ctx := context.WithValue(context.Background(), builder.SettingsKey, builder.Settings{
		ClusterPublicHostname: "meow.com",
	})
	status, err := clientInterface.LeaseStatus(ctx, lid)
	require.NoError(t, err)
	require.NotNil(t, status)

	require.Len(t, status.Services, 2)
	for _, service := range status.Services {
		require.Len(t, service.URIs, 0) // No ingresses, so there should be no URIs
	}
	require.Len(t, status.ForwardedPorts, 1)

	ports := status.ForwardedPorts[serviceName]
	require.Len(t, ports, 1)
	require.Equal(t, int(ports[0].ExternalPort), expectedExternalPort)
}

func TestServiceStatusNoLease(t *testing.T) {
	const serviceName = "foobar"
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	testErr := kubeErrors.NewNotFound(schema.GroupResource{}, "bob")
	require.True(t, kubeErrors.IsNotFound(testErr))
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, testErr)

	clientInterface := clientForTest(t, kmock, nil)

	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
	require.ErrorIs(t, err, ErrLeaseNotFound)
	require.Nil(t, status)
}

func TestServiceStatusNoDeployment(t *testing.T) {
	const serviceName = "foobar"
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)
	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(nil, nil)

	akashMock := akashclient_fake.NewSimpleClientset()

	clientInterface := clientForTest(t, kmock, akashMock)

	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
	require.ErrorIs(t, err, ErrNoDeploymentForLease)
	require.Nil(t, status)
}

func TestServiceStatusNoServiceWithName(t *testing.T) {
	const serviceName = "foobar"
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

	deployment := appsv1.Deployment{}
	deployment.Name = "aname0"
	deployment.Status.AvailableReplicas = 10
	deployment.Status.Replicas = 10

	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)

	mg := &manifest.Group{
		Name:     "somename",
		Services: nil,
	}

	m, err := crd.NewManifest(testKubeClientNs, lid, mg)
	require.NoError(t, err)
	akashMock := akashclient_fake.NewSimpleClientset(m)

	clientInterface := clientForTest(t, kmock, akashMock)

	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
	require.ErrorIs(t, err, ErrNoServiceForLease)
	require.Nil(t, status)
}

func TestServiceStatusNoCRDManifest(t *testing.T) {
	const serviceName = "foobar"
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

	deployment := appsv1.Deployment{}
	deployment.Name = "aname1"
	deployment.Status.AvailableReplicas = 10
	deployment.Status.Replicas = 10

	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)

	mg := &manifest.Group{
		Name:     "somename",
		Services: nil,
	}

	m, err := crd.NewManifest(testKubeClientNs+"a", lid, mg)
	require.NoError(t, err)
	akashMock := akashclient_fake.NewSimpleClientset(m)

	clientInterface := clientForTest(t, kmock, akashMock)

	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
	require.Error(t, err)
	require.Regexp(t, `^manifests.akash.network ".+" not found$`, err)
	require.Nil(t, status)
}

func TestServiceStatusWithIngress(t *testing.T) {
	const serviceName = "foobar"
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

	deployment := appsv1.Deployment{}
	deployment.Name = "aname2"
	deployment.Status.AvailableReplicas = 10
	deployment.Status.Replicas = 10

	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)

	services := make([]manifest.Service, 2)
	services[0] = manifest.Service{
		Name:      "someService",
		Image:     "best/image",
		Command:   nil,
		Args:      nil,
		Env:       nil,
		Resources: types.ResourceUnits{},
		Count:     1,
		Expose: []manifest.ServiceExpose{
			{
				Port:         9000,
				ExternalPort: 9000,
				Proto:        "TCP",
				Service:      "echo",
				Global:       false,
				Hosts:        nil,
			},
		},
	}
	services[1] = manifest.Service{
		Name:      serviceName,
		Image:     "best/image",
		Command:   nil,
		Args:      nil,
		Env:       nil,
		Resources: types.ResourceUnits{},
		Count:     1,
		Expose: []manifest.ServiceExpose{
			{
				Port:         9000,
				ExternalPort: 80,
				Proto:        "TCP",
				Service:      "echo",
				Global:       true,
				Hosts:        []string{"atest.localhost"},
			},
		},
	}
	mg := &manifest.Group{
		Name:     "my-awesome-group",
		Services: services,
	}

	m, err := crd.NewManifest(testKubeClientNs, lid, mg)
	require.NoError(t, err)
	akashMock := akashclient_fake.NewSimpleClientset(m, fakeProviderHost("abcd.com", lid, "echo", 9000))

	clientInterface := clientForTest(t, kmock, akashMock)

	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
	require.NoError(t, err)
	require.NotNil(t, status)

	require.Equal(t, status.URIs, []string{"abcd.com"})
}

func TestServiceStatusWithNoManifest(t *testing.T) {
	const serviceName = "foobar"
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

	deployment := appsv1.Deployment{}
	deployment.Name = "aname4"
	deployment.Status.AvailableReplicas = 10
	deployment.Status.Replicas = 10

	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)

	services := make([]manifest.Service, 2)
	services[0] = manifest.Service{
		Name:      "someService",
		Image:     "best/image",
		Command:   nil,
		Args:      nil,
		Env:       nil,
		Resources: types.ResourceUnits{},
		Count:     1,
		Expose: []manifest.ServiceExpose{
			{
				Port:         9000,
				ExternalPort: 9000,
				Proto:        "TCP",
				Service:      "echo",
				Global:       false,
				Hosts:        nil,
			},
		},
	}
	services[1] = manifest.Service{
		Name:      serviceName,
		Image:     "best/image",
		Command:   nil,
		Args:      nil,
		Env:       nil,
		Resources: types.ResourceUnits{},
		Count:     1,
		Expose: []manifest.ServiceExpose{
			{
				Port:         9000,
				ExternalPort: 80,
				Proto:        "TCP",
				Service:      "echo",
				Global:       true,
				Hosts:        []string{"atest.localhost"},
			},
		},
	}

	akashMock := akashclient_fake.NewSimpleClientset()
	clientInterface := clientForTest(t, kmock, akashMock)

	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
	require.True(t, kubeErrors.IsNotFound(err))
	require.Nil(t, status)
}

func TestServiceStatusWithoutIngress(t *testing.T) {
	const serviceName = "foobar"
	lid := testutil.LeaseID(t)

	kmock := &kubernetes_mocks.Interface{}
	appsV1Mock := &appsv1_mocks.AppsV1Interface{}
	coreV1Mock := &corev1_mocks.CoreV1Interface{}
	kmock.On("AppsV1").Return(appsV1Mock)
	kmock.On("CoreV1").Return(coreV1Mock)

	namespaceMock := &corev1_mocks.NamespaceInterface{}
	coreV1Mock.On("Namespaces").Return(namespaceMock)
	namespaceMock.On("Get", mock.Anything, builder.LidNS(lid), mock.Anything).Return(nil, nil)

	deploymentsMock := &appsv1_mocks.DeploymentInterface{}
	appsV1Mock.On("Deployments", builder.LidNS(lid)).Return(deploymentsMock)

	deployment := appsv1.Deployment{}
	deployment.Name = "aname5"
	deployment.Status.AvailableReplicas = 10
	deployment.Status.Replicas = 10

	deploymentsMock.On("Get", mock.Anything, serviceName, metav1.GetOptions{}).Return(&deployment, nil)

	services := make([]manifest.Service, 2)
	services[0] = manifest.Service{
		Name:      "someService",
		Image:     "best/image",
		Command:   nil,
		Args:      nil,
		Env:       nil,
		Resources: types.ResourceUnits{},
		Count:     1,
		Expose: []manifest.ServiceExpose{
			{
				Port:         9000,
				ExternalPort: 9000,
				Proto:        "TCP",
				Service:      "echo",
				Global:       false,
				Hosts:        nil,
			},
		},
	}
	services[1] = manifest.Service{
		Name:      serviceName,
		Image:     "best/image",
		Command:   nil,
		Args:      nil,
		Env:       nil,
		Resources: types.ResourceUnits{},
		Count:     1,
		Expose: []manifest.ServiceExpose{
			{
				Port:         9000,
				ExternalPort: 80,
				Proto:        "TCP",
				Service:      "echo",
				Global:       false,
				Hosts:        []string{"atest.localhost"},
			},
		},
	}
	mg := &manifest.Group{
		Name:     "my-awesome-group",
		Services: services,
	}

	m, err := crd.NewManifest(testKubeClientNs, lid, mg)
	require.NoError(t, err)
	akashMock := akashclient_fake.NewSimpleClientset(m)

	clientInterface := clientForTest(t, kmock, akashMock)

	status, err := clientInterface.ServiceStatus(context.Background(), lid, serviceName)
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Len(t, status.URIs, 0)
}
