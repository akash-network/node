package kube

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	akashclientfake "github.com/ovrclk/akash/pkg/client/clientset/versioned/fake"
	"github.com/ovrclk/akash/testutil"
	kubernetesmocks "github.com/ovrclk/akash/testutil/kubernetes_mock"
	corev1mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/core/v1"
	storagev1mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/storage/v1"
)

type inventoryScaffold struct {
	kmock                   *kubernetesmocks.Interface
	amock                   *akashclientfake.Clientset
	coreV1Mock              *corev1mocks.CoreV1Interface
	storageV1Interface      *storagev1mocks.StorageV1Interface
	storageClassesInterface *storagev1mocks.StorageClassInterface
	nsInterface             *corev1mocks.NamespaceInterface
	nodeInterfaceMock       *corev1mocks.NodeInterface
	podInterfaceMock        *corev1mocks.PodInterface
	storageClassesList      *storagev1.StorageClassList
	nsList                  *v1.NamespaceList
}

// func fakeStorageClassInfo() runtime.Object {
// labels := make(map[string]string)
// builder.AppendLeaseLabels(leaseID, labels)
// return &akashv1_types.ProviderHost{
// 	TypeMeta: metav1.TypeMeta{},
// 	ObjectMeta: metav1.ObjectMeta{
// 		Name:                       hostname,
// 		GenerateName:               "",
// 		Namespace:                  testKubeClientNs,
// 		UID:                        "",
// 		ResourceVersion:            "",
// 		Generation:                 0,
// 		CreationTimestamp:          metav1.Time{},
// 		DeletionTimestamp:          nil,
// 		DeletionGracePeriodSeconds: nil,
// 		Labels:                     labels,
// 		Annotations:                nil,
// 		OwnerReferences:            nil,
// 		Finalizers:                 nil,
// 		ClusterName:                "",
// 		ManagedFields:              nil,
// 	},
// 	Spec: akashv1_types.ProviderHostSpec{
// 		Owner:        leaseID.Owner,
// 		Provider:     leaseID.Provider,
// 		Hostname:     hostname,
// 		Dseq:         leaseID.DSeq,
// 		Gseq:         leaseID.GSeq,
// 		Oseq:         leaseID.OSeq,
// 		ServiceName:  serviceName,
// 		ExternalPort: externalPort,
// 	},
// 	Status: akashv1_types.ProviderHostStatus{},
// }
// }

func makeInventoryScaffold() *inventoryScaffold {
	s := &inventoryScaffold{
		kmock:                   &kubernetesmocks.Interface{},
		amock:                   akashclientfake.NewSimpleClientset(),
		coreV1Mock:              &corev1mocks.CoreV1Interface{},
		storageV1Interface:      &storagev1mocks.StorageV1Interface{},
		storageClassesInterface: &storagev1mocks.StorageClassInterface{},
		nsInterface:             &corev1mocks.NamespaceInterface{},
		nodeInterfaceMock:       &corev1mocks.NodeInterface{},
		podInterfaceMock:        &corev1mocks.PodInterface{},
		storageClassesList:      &storagev1.StorageClassList{},
		nsList:                  &v1.NamespaceList{},
	}

	s.kmock.On("CoreV1").Return(s.coreV1Mock)

	s.coreV1Mock.On("Namespaces").Return(s.nsInterface, nil)
	s.coreV1Mock.On("Nodes").Return(s.nodeInterfaceMock, nil)
	s.coreV1Mock.On("Pods", "" /* all namespaces */).Return(s.podInterfaceMock, nil)

	s.nsInterface.On("List", mock.Anything, mock.Anything).Return(s.nsList, nil)

	s.kmock.On("StorageV1").Return(s.storageV1Interface)

	s.storageV1Interface.On("StorageClasses").Return(s.storageClassesInterface, nil)
	s.storageClassesInterface.On("List", mock.Anything, mock.Anything).Return(s.storageClassesList, nil)

	return s
}

func TestInventoryZero(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{}
	listOptions := metav1.ListOptions{}
	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nodeList, nil)

	podList := &v1.PodList{}
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inventory, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inventory)

	// The inventory was called and the kubernetes client says there are no nodes & no pods. Inventory
	// should be zero
	require.Len(t, inventory.Metrics().Nodes, 0)

	podListOptionsInCall := s.podInterfaceMock.Calls[0].Arguments[1].(metav1.ListOptions)
	require.Equal(t, "status.phase==Running", podListOptionsInCall.FieldSelector)
}

func TestInventorySingleNodeNoPods(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{}
	nodeList.Items = make([]v1.Node, 1)

	nodeResourceList := make(v1.ResourceList)
	const expectedCPU = 13
	cpuQuantity := resource.NewQuantity(expectedCPU, "m")
	nodeResourceList[v1.ResourceCPU] = *cpuQuantity

	const expectedMemory = 14
	memoryQuantity := resource.NewQuantity(expectedMemory, "M")
	nodeResourceList[v1.ResourceMemory] = *memoryQuantity

	const expectedStorage = 15
	ephemeralStorageQuantity := resource.NewQuantity(expectedStorage, "M")
	nodeResourceList[v1.ResourceEphemeralStorage] = *ephemeralStorageQuantity

	nodeConditions := make([]v1.NodeCondition, 1)
	nodeConditions[0] = v1.NodeCondition{
		Type:   v1.NodeReady,
		Status: v1.ConditionTrue,
	}

	nodeList.Items[0] = v1.Node{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       v1.NodeSpec{},
		Status: v1.NodeStatus{
			Allocatable: nodeResourceList,
			Conditions:  nodeConditions,
		},
	}

	listOptions := metav1.ListOptions{}
	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nodeList, nil)

	podList := &v1.PodList{}
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inventory, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inventory)

	require.Len(t, inventory.Metrics().Nodes, 1)

	node := inventory.Metrics().Nodes[0]
	availableResources := node.Available
	// Multiply expected value by 1000 since millicpu is used
	require.Equal(t, uint64(expectedCPU*1000), availableResources.CPU)
	require.Equal(t, uint64(expectedMemory), availableResources.Memory)
	require.Equal(t, uint64(expectedStorage), availableResources.StorageEphemeral)
}

func TestInventorySingleNodeWithPods(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{}
	nodeList.Items = make([]v1.Node, 1)

	nodeResourceList := make(v1.ResourceList)
	const expectedCPU = 13
	cpuQuantity := resource.NewQuantity(expectedCPU, "m")
	nodeResourceList[v1.ResourceCPU] = *cpuQuantity

	const expectedMemory = 2048
	memoryQuantity := resource.NewQuantity(expectedMemory, "M")
	nodeResourceList[v1.ResourceMemory] = *memoryQuantity

	const expectedStorage = 4096
	ephemeralStorageQuantity := resource.NewQuantity(expectedStorage, "M")
	nodeResourceList[v1.ResourceEphemeralStorage] = *ephemeralStorageQuantity

	nodeConditions := make([]v1.NodeCondition, 1)
	nodeConditions[0] = v1.NodeCondition{
		Type:   v1.NodeReady,
		Status: v1.ConditionTrue,
	}

	nodeList.Items[0] = v1.Node{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       v1.NodeSpec{},
		Status: v1.NodeStatus{
			Allocatable: nodeResourceList,
			Conditions:  nodeConditions,
		},
	}

	listOptions := metav1.ListOptions{}
	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nodeList, nil)

	const cpuPerContainer = 1
	const memoryPerContainer = 3
	const storagePerContainer = 17
	// Define two pods
	pods := make([]v1.Pod, 2)
	// First pod has 1 container
	podContainers := make([]v1.Container, 1)
	containerRequests := make(v1.ResourceList)
	cpuQuantity.SetMilli(cpuPerContainer)
	containerRequests[v1.ResourceCPU] = *cpuQuantity

	memoryQuantity = resource.NewQuantity(memoryPerContainer, "M")
	containerRequests[v1.ResourceMemory] = *memoryQuantity

	ephemeralStorageQuantity = resource.NewQuantity(storagePerContainer, "M")
	containerRequests[v1.ResourceEphemeralStorage] = *ephemeralStorageQuantity

	podContainers[0] = v1.Container{
		Resources: v1.ResourceRequirements{
			Limits:   nil,
			Requests: containerRequests,
		},
	}
	pods[0] = v1.Pod{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: v1.PodSpec{
			Containers: podContainers,
		},
		Status: v1.PodStatus{},
	}

	// Define 2nd pod with multiple containers
	podContainers = make([]v1.Container, 2)
	for i := range podContainers {
		containerRequests := make(v1.ResourceList)
		cpuQuantity.SetMilli(cpuPerContainer)
		containerRequests[v1.ResourceCPU] = *cpuQuantity

		memoryQuantity = resource.NewQuantity(memoryPerContainer, "M")
		containerRequests[v1.ResourceMemory] = *memoryQuantity

		ephemeralStorageQuantity = resource.NewQuantity(storagePerContainer, "M")
		containerRequests[v1.ResourceEphemeralStorage] = *ephemeralStorageQuantity

		// Container limits are enforced by kubernetes as absolute limits, but not
		// used when considering inventory since overcommit is possible in a kubernetes cluster
		// Set limits to any value larger than requests in this test since it should not change
		// the value returned  by the code
		containerLimits := make(v1.ResourceList)

		for k, v := range containerRequests {
			replacementV := resource.NewQuantity(0, "")
			replacementV.Set(v.Value() * int64(testutil.RandRangeInt(2, 100)))
			containerLimits[k] = *replacementV
		}

		podContainers[i] = v1.Container{
			Resources: v1.ResourceRequirements{
				Limits:   containerLimits,
				Requests: containerRequests,
			},
		}
	}
	pods[1] = v1.Pod{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: v1.PodSpec{
			Containers: podContainers,
		},
		Status: v1.PodStatus{},
	}

	podList := &v1.PodList{
		Items: pods,
	}

	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inventory, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inventory)

	require.Len(t, inventory.Metrics().Nodes, 1)

	node := inventory.Metrics().Nodes[0]
	availableResources := node.Available
	// Multiply expected value by 1000 since millicpu is used
	require.Equal(t, uint64(expectedCPU*1000)-3*cpuPerContainer, availableResources.CPU)
	require.Equal(t, uint64(expectedMemory)-3*memoryPerContainer, availableResources.Memory)
	require.Equal(t, uint64(expectedStorage)-3*storagePerContainer, availableResources.StorageEphemeral)
}

var errForTest = errors.New("error in test")

func TestInventoryWithNodeError(t *testing.T) {
	s := makeInventoryScaffold()

	listOptions := metav1.ListOptions{}
	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nil, errForTest)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inventory, err := clientInterface.Inventory(context.Background())
	require.Error(t, err)
	require.True(t, errors.Is(err, errForTest))
	require.Nil(t, inventory)
}

func TestInventoryWithPodsError(t *testing.T) {
	s := makeInventoryScaffold()

	listOptions := metav1.ListOptions{}
	nodeList := &v1.NodeList{}
	s.nodeInterfaceMock.On("List", mock.Anything, listOptions).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nil, errForTest)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inventory, err := clientInterface.Inventory(context.Background())
	require.Error(t, err)
	require.True(t, errors.Is(err, errForTest))
	require.Nil(t, inventory)
}
