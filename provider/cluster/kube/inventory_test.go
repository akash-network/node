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
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/testutil"
	kubernetesmocks "github.com/ovrclk/akash/testutil/kubernetes_mock"
	corev1mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/core/v1"
	storagev1mocks "github.com/ovrclk/akash/testutil/kubernetes_mock/typed/storage/v1"
	"github.com/ovrclk/akash/types/unit"
	atypes "github.com/ovrclk/akash/types/v1beta2"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type testReservation struct {
	resources dtypes.GroupSpec
}

var _ ctypes.Reservation = (*testReservation)(nil)

func (r *testReservation) OrderID() mtypes.OrderID {
	return mtypes.OrderID{}
}

func (r *testReservation) Resources() atypes.ResourceGroup {
	return r.resources
}

func (r *testReservation) Allocated() bool {
	return false
}

type inventoryScaffold struct {
	kmock                   *kubernetesmocks.Interface
	amock                   *akashclientfake.Clientset
	coreV1Mock              *corev1mocks.CoreV1Interface
	storageV1Interface      *storagev1mocks.StorageV1Interface
	storageClassesInterface *storagev1mocks.StorageClassInterface
	nsInterface             *corev1mocks.NamespaceInterface
	nodeInterfaceMock       *corev1mocks.NodeInterface
	podInterfaceMock        *corev1mocks.PodInterface
	servicesInterfaceMock   *corev1mocks.ServiceInterface
	storageClassesList      *storagev1.StorageClassList
	nsList                  *v1.NamespaceList
}

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
		servicesInterfaceMock:   &corev1mocks.ServiceInterface{},
		storageClassesList:      &storagev1.StorageClassList{},
		nsList:                  &v1.NamespaceList{},
	}

	s.kmock.On("CoreV1").Return(s.coreV1Mock)

	s.coreV1Mock.On("Namespaces").Return(s.nsInterface, nil)
	s.coreV1Mock.On("Nodes").Return(s.nodeInterfaceMock, nil)
	s.coreV1Mock.On("Pods", "" /* all namespaces */).Return(s.podInterfaceMock, nil)
	s.coreV1Mock.On("Services", "" /* all namespaces */).Return(s.servicesInterfaceMock, nil)

	s.nsInterface.On("List", mock.Anything, mock.Anything).Return(s.nsList, nil)

	s.kmock.On("StorageV1").Return(s.storageV1Interface)

	s.storageV1Interface.On("StorageClasses").Return(s.storageClassesInterface, nil)
	s.storageClassesInterface.On("List", mock.Anything, mock.Anything).Return(s.storageClassesList, nil)

	s.servicesInterfaceMock.On("List", mock.Anything, mock.Anything).Return(&v1.ServiceList{}, nil)

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

func TestInventoryMultipleReplicasFulFilled1(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{
		Items: multipleReplicasGenNodes(),
	}

	podList := &v1.PodList{Items: []v1.Pod{}}

	s.nodeInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inv, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Len(t, inv.Metrics().Nodes, 4)

	err = inv.Adjust(multipleReplicasGenReservations(100000, 2))
	require.NoError(t, err)
}

func TestInventoryMultipleReplicasFulFilled2(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{
		Items: multipleReplicasGenNodes(),
	}

	podList := &v1.PodList{Items: []v1.Pod{}}

	s.nodeInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inv, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Len(t, inv.Metrics().Nodes, 4)

	err = inv.Adjust(multipleReplicasGenReservations(68780, 4))
	require.NoError(t, err)
}

func TestInventoryMultipleReplicasFulFilled3(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{
		Items: multipleReplicasGenNodes(),
	}

	podList := &v1.PodList{Items: []v1.Pod{}}

	s.nodeInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inv, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Len(t, inv.Metrics().Nodes, 4)

	err = inv.Adjust(multipleReplicasGenReservations(68800, 3))
	require.NoError(t, err)
}

func TestInventoryMultipleReplicasFulFilled4(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{
		Items: multipleReplicasGenNodes(),
	}

	podList := &v1.PodList{Items: []v1.Pod{}}

	s.nodeInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inv, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Len(t, inv.Metrics().Nodes, 4)

	err = inv.Adjust(multipleReplicasGenReservations(119495, 2))
	require.NoError(t, err)
}

func TestInventoryMultipleReplicasFulFilled5(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{
		Items: multipleReplicasGenNodes(),
	}

	podList := &v1.PodList{Items: []v1.Pod{}}

	s.nodeInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inv, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Len(t, inv.Metrics().Nodes, 4)

	err = inv.Adjust(multipleReplicasGenReservations(68780, 1))
	require.NoError(t, err)
}

func TestInventoryMultipleReplicasOutOfCapacity1(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{
		Items: multipleReplicasGenNodes(),
	}

	podList := &v1.PodList{Items: []v1.Pod{}}

	s.nodeInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inv, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Len(t, inv.Metrics().Nodes, 4)

	err = inv.Adjust(multipleReplicasGenReservations(70000, 4))
	require.Error(t, err)
	require.EqualError(t, ctypes.ErrInsufficientCapacity, err.Error())
}

func TestInventoryMultipleReplicasOutOfCapacity2(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{
		Items: multipleReplicasGenNodes(),
	}

	podList := &v1.PodList{Items: []v1.Pod{}}

	s.nodeInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inv, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Len(t, inv.Metrics().Nodes, 4)

	err = inv.Adjust(multipleReplicasGenReservations(100000, 3))
	require.Error(t, err)
	require.EqualError(t, ctypes.ErrInsufficientCapacity, err.Error())
}

func TestInventoryMultipleReplicasOutOfCapacity4(t *testing.T) {
	s := makeInventoryScaffold()

	nodeList := &v1.NodeList{
		Items: multipleReplicasGenNodes(),
	}

	podList := &v1.PodList{Items: []v1.Pod{}}

	s.nodeInterfaceMock.On("List", mock.Anything, mock.Anything).Return(nodeList, nil)
	s.podInterfaceMock.On("List", mock.Anything, mock.Anything).Return(podList, nil)

	clientInterface := clientForTest(t, s.kmock, s.amock)
	inv, err := clientInterface.Inventory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Len(t, inv.Metrics().Nodes, 4)

	err = inv.Adjust(multipleReplicasGenReservations(119525, 2))
	require.Error(t, err)
	require.EqualError(t, ctypes.ErrInsufficientCapacity, err.Error())
}

// multipleReplicasGenNodes generates four nodes with following CPUs available
//   node1: 68780
//   node2: 68800
//   node3: 119525
//   node4: 119495
func multipleReplicasGenNodes() []v1.Node {
	nodeCapacity := make(v1.ResourceList)
	nodeCapacity[v1.ResourceCPU] = *(resource.NewMilliQuantity(119800, resource.DecimalSI))
	nodeCapacity[v1.ResourceMemory] = *(resource.NewQuantity(474813259776, resource.DecimalSI))
	nodeCapacity[v1.ResourceEphemeralStorage] = *(resource.NewQuantity(7760751097705, resource.DecimalSI))

	nodeConditions := make([]v1.NodeCondition, 1)
	nodeConditions[0] = v1.NodeCondition{
		Type:   v1.NodeReady,
		Status: v1.ConditionTrue,
	}

	return []v1.Node{
		{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Spec: v1.NodeSpec{},
			Status: v1.NodeStatus{
				Allocatable: v1.ResourceList{
					v1.ResourceCPU:              *(resource.NewMilliQuantity(68780, resource.DecimalSI)),
					v1.ResourceMemory:           *(resource.NewQuantity(457317732352, resource.DecimalSI)),
					v1.ResourceEphemeralStorage: *(resource.NewQuantity(7752161163113, resource.DecimalSI)),
				},
				Capacity:   nodeCapacity,
				Conditions: nodeConditions,
			},
		},
		{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
			},
			Spec: v1.NodeSpec{},
			Status: v1.NodeStatus{
				Allocatable: v1.ResourceList{
					v1.ResourceCPU:              *(resource.NewMilliQuantity(68800, resource.DecimalSI)),
					v1.ResourceMemory:           *(resource.NewQuantity(457328218112, resource.DecimalSI)),
					v1.ResourceEphemeralStorage: *(resource.NewQuantity(7752161163113, resource.DecimalSI)),
				},
				Capacity:   nodeCapacity,
				Conditions: nodeConditions,
			},
		},
		{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: "node3",
			},
			Spec: v1.NodeSpec{},
			Status: v1.NodeStatus{
				Allocatable: v1.ResourceList{
					v1.ResourceCPU:              *(resource.NewMilliQuantity(119525, resource.DecimalSI)),
					v1.ResourceMemory:           *(resource.NewQuantity(474817923072, resource.DecimalSI)),
					v1.ResourceEphemeralStorage: *(resource.NewQuantity(7760751097705, resource.DecimalSI)),
				},
				Capacity:   nodeCapacity,
				Conditions: nodeConditions,
			},
		},
		{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: "node4",
			},
			Spec: v1.NodeSpec{},
			Status: v1.NodeStatus{
				Allocatable: v1.ResourceList{
					v1.ResourceCPU:              *(resource.NewMilliQuantity(119495, resource.DecimalSI)),
					v1.ResourceMemory:           *(resource.NewQuantity(474753923072, resource.DecimalSI)),
					v1.ResourceEphemeralStorage: *(resource.NewQuantity(7760751097705, resource.DecimalSI)),
				},
				Capacity:   nodeCapacity,
				Conditions: nodeConditions,
			},
		},
	}
}

func multipleReplicasGenReservations(cpuUnits uint64, count uint32) *testReservation {
	return &testReservation{
		resources: dtypes.GroupSpec{
			Name:         "bla",
			Requirements: atypes.PlacementRequirements{},
			Resources: []dtypes.Resource{
				{
					Resources: atypes.ResourceUnits{
						CPU: &atypes.CPU{
							Units: atypes.NewResourceValue(cpuUnits),
						},
						Memory: &atypes.Memory{
							Quantity: atypes.NewResourceValue(16 * unit.Gi),
						},
						Storage: []atypes.Storage{
							{
								Name:     "default",
								Quantity: atypes.NewResourceValue(8 * unit.Gi),
							},
						},
					},
					Count: count,
				},
			},
		},
	}
}
