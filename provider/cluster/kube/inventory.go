package kube

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/pager"

	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/sdl"
	types "github.com/ovrclk/akash/types/v1beta2"
	metricsutils "github.com/ovrclk/akash/util/metrics"
)

type node struct {
	id               string
	arch             string
	cpu              resourcePair
	memory           resourcePair
	ephemeralStorage resourcePair
	volumesAttached  resourcePair
	volumesMounted   resourcePair
	storageClasses   map[string]bool
}

type clusterNodes map[string]*node

type inventory struct {
	storageClasses clusterStorage
	nodes          clusterNodes
}

var _ ctypes.Inventory = (*inventory)(nil)

func newInventory(storage clusterStorage, nodes map[string]*node) *inventory {
	inv := &inventory{
		storageClasses: storage,
		nodes:          nodes,
	}

	return inv
}

func (inv *inventory) dup() inventory {
	dup := inventory{
		storageClasses: inv.storageClasses.dup(),
		nodes:          inv.nodes.dup(),
	}

	return dup
}

func (nd *node) allowsStorageClasses(volumes types.Volumes) bool {
	for _, storage := range volumes {
		attr := storage.Attributes.Find(sdl.StorageAttributePersistent)
		if persistent, set := attr.AsBool(); !set || !persistent {
			continue
		}

		attr = storage.Attributes.Find(sdl.StorageAttributeClass)
		if class, set := attr.AsString(); set {
			if _, allowed := nd.storageClasses[class]; !allowed {
				return false
			}
		}
	}

	return true
}

func (inv *inventory) Adjust(reservation ctypes.Reservation) error {
	resources := make([]types.Resources, len(reservation.Resources().GetResources()))
	copy(resources, reservation.Resources().GetResources())

	currInventory := inv.dup()

nodes:
	for nodeName, nd := range currInventory.nodes {
		currResources := resources[:0]

		for _, res := range resources {
			for ; res.Count > 0; res.Count-- {
				// first check if there reservation needs persistent storage
				// and node handles such class
				if !nd.allowsStorageClasses(res.Resources.Storage) {
					continue nodes
				}

				var adjusted bool

				cpu := nd.cpu.dup()
				if adjusted = cpu.subMilliNLZ(res.Resources.CPU.Units); !adjusted {
					continue nodes
				}

				memory := nd.memory.dup()
				if adjusted = memory.subNLZ(res.Resources.Memory.Quantity); !adjusted {
					continue nodes
				}

				ephemeralStorage := nd.ephemeralStorage.dup()
				volumesAttached := nd.volumesAttached.dup()

				storageClasses := currInventory.storageClasses.dup()

				for _, storage := range res.Resources.Storage {
					attr := storage.Attributes.Find(sdl.StorageAttributePersistent)

					if persistent, _ := attr.AsBool(); !persistent {
						if adjusted = ephemeralStorage.subNLZ(storage.Quantity); !adjusted {
							continue nodes
						}
						continue
					}

					// if volumesAttached, adjusted = volumesAttached.subNLZ(types.NewResourceValue(1)); !adjusted {
					// 	continue nodes
					// }

					attr = storage.Attributes.Find(sdl.StorageAttributeClass)
					class, _ := attr.AsString()

					cstorage, isAvailable := storageClasses[class]
					if !isAvailable {
						break nodes
					}

					if adjusted = cstorage.subNLZ(storage.Quantity); !adjusted {
						// cluster storage does not have enough space thus break to error
						break nodes
					}
				}

				// all requirements for current group have been satisfied
				// commit and move on
				currInventory.nodes[nodeName] = &node{
					id:               nd.id,
					arch:             nd.arch,
					cpu:              *cpu,
					memory:           *memory,
					ephemeralStorage: *ephemeralStorage,
					volumesAttached:  *volumesAttached,
					volumesMounted:   nd.volumesMounted,
					storageClasses:   nd.storageClasses,
				}

				currInventory.storageClasses = storageClasses
			}

			if res.Count > 0 {
				currResources = append(currResources, res)
			}
		}

		resources = currResources
	}

	if len(resources) == 0 {
		*inv = currInventory
		return nil
	}

	return ctypes.ErrInsufficientCapacity
}

func (inv *inventory) Metrics() ctypes.InventoryMetrics {
	cpuTotal := uint64(0)
	memoryTotal := uint64(0)
	storageEphemeralTotal := uint64(0)
	storageTotal := make(map[string]int64)

	cpuAvailable := uint64(0)
	memoryAvailable := uint64(0)
	storageEphemeralAvailable := uint64(0)
	storageAvailable := make(map[string]int64)

	ret := ctypes.InventoryMetrics{
		Nodes: make([]ctypes.InventoryNode, 0, len(inv.nodes)),
	}

	for nodeName, nd := range inv.nodes {
		invNode := ctypes.InventoryNode{
			Name: nodeName,
			Allocatable: ctypes.InventoryNodeMetric{
				CPU:              uint64(nd.cpu.allocatable.MilliValue()),
				Memory:           uint64(nd.memory.allocatable.Value()),
				StorageEphemeral: uint64(nd.ephemeralStorage.allocatable.Value()),
			},
		}

		cpuTotal += uint64(nd.cpu.allocatable.MilliValue())
		memoryTotal += uint64(nd.memory.allocatable.Value())
		storageEphemeralTotal += uint64(nd.ephemeralStorage.allocatable.Value())

		avail := nd.cpu.available()
		invNode.Available.CPU = uint64(avail.MilliValue())
		cpuAvailable += invNode.Available.CPU

		avail = nd.memory.available()
		invNode.Available.Memory = uint64(avail.Value())
		memoryAvailable += invNode.Available.Memory

		avail = nd.ephemeralStorage.available()
		invNode.Available.StorageEphemeral = uint64(avail.Value())
		storageEphemeralAvailable += invNode.Available.StorageEphemeral

		ret.Nodes = append(ret.Nodes, invNode)
	}

	for class, storage := range inv.storageClasses {
		tmp := storage.allocatable.DeepCopy()
		storageTotal[class] = tmp.Value()

		tmp = storage.available()
		storageAvailable[class] = tmp.Value()
	}

	ret.TotalAllocatable = ctypes.InventoryMetricTotal{
		CPU:              cpuTotal,
		Memory:           memoryTotal,
		StorageEphemeral: storageEphemeralTotal,
		Storage:          storageTotal,
	}

	ret.TotalAvailable = ctypes.InventoryMetricTotal{
		CPU:              cpuAvailable,
		Memory:           memoryAvailable,
		StorageEphemeral: storageEphemeralAvailable,
		Storage:          storageAvailable,
	}

	return ret
}

func (c *client) Inventory(ctx context.Context) (ctypes.Inventory, error) {
	cstorage, err := c.fetchStorage(ctx)
	if err != nil {
		// log inventory operator error but keep going to fetch nodes
		// as provider still may make bids on orders without persistent storage
		c.log.Error("checking storage inventory", "error", err.Error())
	}

	knodes, err := c.fetchActiveNodes(ctx, cstorage)
	if err != nil {
		return nil, err
	}

	return newInventory(cstorage, knodes), nil
}

func (c *client) fetchStorage(ctx context.Context) (clusterStorage, error) {
	cstorage := make(clusterStorage)

	// TODO - figure out if we can get this back via ServiceDiscoveryAgent query akash-services inventory-operator api

	// discover inventory operator
	// empty namespace mean search through all namespaces
	svcResult, err := c.kc.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: builder.AkashManagedLabelName + "=true" +
			",app.kubernetes.io/name=akash" +
			",app.kubernetes.io/instance=inventory" +
			",app.kubernetes.io/component=operator",
	})
	if err != nil {
		return nil, err
	}

	if len(svcResult.Items) == 0 {
		return nil, nil
	}

	result := c.kc.CoreV1().RESTClient().Get().
		Namespace(svcResult.Items[0].Namespace).
		Resource("services").
		Name(svcResult.Items[0].Name + ":api").
		SubResource("proxy").
		Suffix("inventory").
		Do(ctx)

	if err := result.Error(); err != nil {
		return nil, err
	}

	inv := &crd.Inventory{}

	if err := result.Into(inv); err != nil {
		return nil, err
	}

	statusPairs := make([]interface{}, 0, len(inv.Status.Messages))
	for idx, msg := range inv.Status.Messages {
		statusPairs = append(statusPairs, fmt.Sprintf("msg%d", idx))
		statusPairs = append(statusPairs, msg)
	}

	if len(statusPairs) > 0 {
		c.log.Info("inventory request performed with warnings", statusPairs...)
	}

	for _, storage := range inv.Spec.Storage {
		if !isSupportedStorageClass(storage.Class) {
			continue
		}

		cstorage[storage.Class] = rpNewFromAkash(storage.ResourcePair)
	}

	return cstorage, nil
}

func (c *client) fetchActiveNodes(ctx context.Context, cstorage clusterStorage) (map[string]*node, error) {
	// todo filter nodes by akash.network label
	knodes, err := c.kc.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("nodes-list", label).Inc()
	if err != nil {
		return nil, err
	}

	podListOptions := metav1.ListOptions{
		FieldSelector: "status.phase==Running",
	}
	podsClient := c.kc.CoreV1().Pods(metav1.NamespaceAll)
	podsPager := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return podsClient.List(ctx, opts)
	})
	zero := resource.NewMilliQuantity(0, "m")

	retnodes := make(map[string]*node)
	for _, knode := range knodes.Items {
		if !c.nodeIsActive(knode) {
			continue
		}

		// Create an entry with the allocatable amount for the node
		cpu := knode.Status.Allocatable.Cpu().DeepCopy()
		memory := knode.Status.Allocatable.Memory().DeepCopy()
		storage := knode.Status.Allocatable.StorageEphemeral().DeepCopy()
		entry := &node{
			arch: knode.Status.NodeInfo.Architecture,
			cpu: resourcePair{
				allocatable: cpu,
			},
			memory: resourcePair{
				allocatable: memory,
			},
			ephemeralStorage: resourcePair{
				allocatable: storage,
			},
			volumesAttached: resourcePair{
				allocated: *resource.NewQuantity(int64(len(knode.Status.VolumesAttached)), resource.DecimalSI),
			},
			storageClasses: make(map[string]bool),
		}

		if value, defined := knode.Labels[builder.AkashNetworkStorageClasses]; defined {
			for _, class := range strings.Split(value, ".") {
				if _, avail := cstorage[class]; avail {
					entry.storageClasses[class] = true
				} else {
					c.log.Info("skipping inactive storage class requested by", "node", knode.Name, "storageClass", class)
				}
			}
		}

		// Initialize the allocated amount to for each node
		zero.DeepCopyInto(&entry.cpu.allocated)
		zero.DeepCopyInto(&entry.memory.allocated)
		zero.DeepCopyInto(&entry.ephemeralStorage.allocated)

		retnodes[knode.Name] = entry
	}

	// Go over each pod and sum the resources for it into the value for the pod it lives on
	err = podsPager.EachListItem(ctx, podListOptions, func(obj runtime.Object) error {
		pod := obj.(*corev1.Pod)
		nodeName := pod.Spec.NodeName

		entry, validNode := retnodes[nodeName]
		if !validNode {
			return nil
		}

		for _, container := range pod.Spec.Containers {
			entry.addAllocatedResources(container.Resources.Requests)
		}

		// Add overhead for running a pod to the sum of requests
		// https://kubernetes.io/docs/concepts/scheduling-eviction/pod-overhead/
		entry.addAllocatedResources(pod.Spec.Overhead)

		retnodes[nodeName] = entry // Map is by value, so store the copy back into the map
		return nil
	})

	if err != nil {
		return nil, err
	}

	return retnodes, nil
}

func (nd *node) addAllocatedResources(rl corev1.ResourceList) {
	for name, quantity := range rl {
		switch name {
		case corev1.ResourceCPU:
			nd.cpu.allocated.Add(quantity)
		case corev1.ResourceMemory:
			nd.memory.allocated.Add(quantity)
		case corev1.ResourceEphemeralStorage:
			nd.ephemeralStorage.allocated.Add(quantity)
		}
	}
}

func (nd *node) dup() *node {
	res := &node{
		id:               nd.id,
		arch:             nd.arch,
		cpu:              *nd.cpu.dup(),
		memory:           *nd.memory.dup(),
		ephemeralStorage: *nd.ephemeralStorage.dup(),
		volumesAttached:  *nd.volumesAttached.dup(),
		volumesMounted:   *nd.volumesMounted.dup(),
		storageClasses:   make(map[string]bool),
	}

	for k, v := range nd.storageClasses {
		res.storageClasses[k] = v
	}

	return res
}

func (cn clusterNodes) dup() clusterNodes {
	ret := make(clusterNodes)

	for name, nd := range cn {
		ret[name] = nd.dup()
	}
	return ret
}
func (c *client) nodeIsActive(node corev1.Node) bool {
	ready := false
	issues := 0

	for _, cond := range node.Status.Conditions {
		switch cond.Type {
		case corev1.NodeReady:
			if cond.Status == corev1.ConditionTrue {
				ready = true
			}
		case corev1.NodeMemoryPressure:
			fallthrough
		case corev1.NodeDiskPressure:
			fallthrough
		case corev1.NodePIDPressure:
			fallthrough
		case corev1.NodeNetworkUnavailable:
			if cond.Status != corev1.ConditionFalse {
				c.log.Error("node in poor condition",
					"node", node.Name,
					"condition", cond.Type,
					"status", cond.Status)

				issues++
			}
		}
	}

	// If the node has been tainted, don't consider it active.
	for _, taint := range node.Spec.Taints {
		if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
			issues++
			c.log.Error("node in poor condition due to active taint",
				"node", node.Name,
				"key", taint.Key,
				"effect", taint.Effect)
		}
	}

	return ready && issues == 0
}

func isSupportedStorageClass(name string) bool {
	switch name {
	case "default":
		fallthrough
	case "beta1":
		fallthrough
	case "beta2":
		fallthrough
	case "beta3":
		return true
	default:
		return false
	}
}
