package kube

import (
	"context"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/pager"

	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/types"
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

var akashManagedOpts = metav1.ListOptions{
	LabelSelector: builder.AkashManagedLabelName + "=true",
}

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
				if cpu, adjusted = cpu.subMilliNLZ(res.Resources.CPU.Units); !adjusted {
					continue nodes
				}

				memory := nd.memory.dup()
				if memory, adjusted = memory.subNLZ(res.Resources.Memory.Quantity); !adjusted {
					continue nodes
				}

				ephemeralStorage := nd.ephemeralStorage.dup()
				volumesAttached := nd.volumesAttached.dup()

				storageClasses := currInventory.storageClasses.dup()

				for idx, storage := range res.Resources.Storage {
					attr := storage.Attributes.Find(sdl.StorageAttributePersistent)

					// no mount point in storage entry is set for ephemeral
					if persistent, _ := attr.AsBool(); !persistent {
						if ephemeralStorage, adjusted = ephemeralStorage.subNLZ(storage.Quantity); !adjusted {
							continue nodes
						}
						continue
					}

					attr = storage.Attributes.Find(sdl.StorageAttributeClass)
					class, _ := attr.AsString()

					// if volumesAttached, adjusted = volumesAttached.subNLZ(types.NewResourceValue(1)); !adjusted {
					// 	continue nodes
					// }

					if class == sdl.StorageClassDefault {
						for name, params := range storageClasses {
							if params.isDefault {
								class = name

								for i := range storage.Attributes {
									if storage.Attributes[i].Key == sdl.StorageAttributeClass {
										res.Resources.Storage[idx].Attributes[i].Value = class
										break
									}
								}
								break
							}
						}
					}

					cstorage, activeStorageClass := storageClasses[class]
					if !activeStorageClass {
						continue nodes
					}

					if _, adjusted = cstorage.subNLZ(storage.Quantity); !adjusted {
						// cluster storage does not have enough space thus break to error
						break nodes
					}
				}

				// all requirements for current group have been satisfied
				// commit and move on
				currInventory.nodes[nodeName] = &node{
					id:               nd.id,
					arch:             nd.arch,
					cpu:              cpu,
					memory:           memory,
					ephemeralStorage: ephemeralStorage,
					volumesAttached:  volumesAttached,
					volumesMounted:   nd.volumesMounted,
					storageClasses:   nd.storageClasses,
				}
			}

			// retResources = append(retResources, currResources...)

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
	cpuTotal := 0.0
	memoryTotal := uint64(0)
	storageEphemeralTotal := uint64(0)
	storageTotal := make(map[string]uint64)

	cpuAvailable := 0.0
	memoryAvailable := uint64(0)
	storageEphemeralAvailable := uint64(0)
	storageAvailable := make(map[string]uint64)

	ret := ctypes.InventoryMetrics{
		Nodes: make([]ctypes.InventoryNode, 0, len(inv.nodes)),
	}

	for nodeName, nd := range inv.nodes {
		invNode := ctypes.InventoryNode{
			Name: nodeName,
			Allocatable: ctypes.InventoryNodeMetric{
				CPU:              float64(nd.cpu.allocatable.MilliValue()) / 1000,
				Memory:           uint64(nd.memory.allocatable.Value()),
				StorageEphemeral: uint64(nd.ephemeralStorage.allocatable.Value()),
			},
		}

		cpuTotal += float64(nd.cpu.allocatable.MilliValue()) / 1000
		memoryTotal += uint64(nd.memory.allocatable.Value())
		storageEphemeralTotal += uint64(nd.ephemeralStorage.allocatable.Value())

		tmp := nd.cpu.allocatable.DeepCopy()
		tmp.Sub(nd.cpu.allocated)
		invNode.Available.CPU = float64(tmp.MilliValue()) / 1000
		cpuAvailable += invNode.Available.CPU

		tmp = nd.memory.allocatable.DeepCopy()
		tmp.Sub(nd.memory.allocated)
		invNode.Available.Memory = uint64(tmp.Value())
		memoryAvailable += invNode.Available.Memory

		tmp = nd.ephemeralStorage.allocatable.DeepCopy()
		tmp.Sub(nd.ephemeralStorage.allocated)
		invNode.Available.StorageEphemeral = uint64(tmp.Value())
		storageEphemeralAvailable += invNode.Available.StorageEphemeral

		ret.Nodes = append(ret.Nodes, invNode)
	}

	for class, storage := range inv.storageClasses {
		tmp := storage.allocatable.DeepCopy()
		storageTotal[class] = uint64(tmp.Value())

		tmp.Sub(storage.allocated)
		storageAvailable[class] = uint64(tmp.Value())
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
	cstorage, classes, err := c.fetchClusterStorage(ctx)
	if err != nil {
		return nil, err
	}

	knodes, err := c.fetchActiveNodes(ctx, cstorage)
	if err != nil {
		return nil, err
	}

	defer c.storageClassesLock.Unlock()
	c.storageClassesLock.Lock()
	c.currStorageClasses = classes

	return newInventory(cstorage, knodes), nil
}

func (c *client) fetchClusterStorage(ctx context.Context) (clusterStorage, map[string]bool, error) {
	currClasses, err := c.fetchStorageClasses(ctx)
	if err != nil {
		return nil, nil, err
	}

	allocated, err := c.fetchAllocatedStorage(ctx)
	if err != nil {
		return nil, nil, err
	}

	activeClasses := make(map[string]bool)
	for name := range currClasses {
		activeClasses[name] = true
	}

	for class := range allocated {
		if _, active := activeClasses[class]; !active {
			activeClasses[class] = false
		}
	}

	storage := make(clusterStorage)

	for class, isDefault := range currClasses {
		storage[class] = &storageClassState{
			isActive:  activeClasses[class],
			isDefault: isDefault,
			resourcePair: resourcePair{
				allocatable: *resource.NewQuantity(-1, resource.DecimalSI),
			},
		}

		if val, isAllocated := allocated[class]; isAllocated {
			storage[class].allocated = *val
		}
	}

	storageInfo, err := c.ac.AkashV1().StorageClassInfos().List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, err
	}

	for _, state := range storageInfo.Items {
		storage[state.Name].allocatable = *resource.NewQuantity(int64(state.Spec.Capacity), resource.DecimalSI)
	}

	return storage, currClasses, nil
}

func (c *client) fetchStorageClasses(ctx context.Context) (map[string]bool, error) {
	sc, err := c.kc.StorageV1().StorageClasses().List(ctx, akashManagedOpts)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	classes := make(map[string]bool)

	for _, ksclass := range sc.Items {
		if !isSupportedStorageClass(ksclass.Name) {
			continue
		}

		isDefault := false

		if val, set := ksclass.Annotations["storageclass.kubernetes.io/is-default-class"]; set {
			if isDefault, err = strconv.ParseBool(val); err != nil {
				c.log.Error("unable to parse value of \"storageclass.kubernetes.io/is-default-class\" annotation", "error", err.Error())
			}
		}

		classes[ksclass.Name] = isDefault
	}

	return classes, nil
}

func (c *client) fetchAllocatedStorage(ctx context.Context) (map[string]*resource.Quantity, error) {
	res := make(map[string]*resource.Quantity)

	nsList, err := c.kc.CoreV1().Namespaces().List(ctx, akashManagedOpts)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	// list all persistent volumes dedicated to akash
	for _, ns := range nsList.Items {
		pvcs, err := c.kc.CoreV1().PersistentVolumeClaims(ns.Name).List(ctx, akashManagedOpts)
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}

		for _, pvc := range pvcs.Items {
			// count only claims with storageClass
			if class := pvc.Spec.StorageClassName; class != nil && isSupportedStorageClass(*class) {
				val, exists := res[*class]
				if !exists {
					val = resource.NewQuantity(0, resource.DecimalSI)
					res[*class] = val
				}

				val.Add(*pvc.Spec.Resources.Requests.Storage())
			}
		}
	}

	return res, nil
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
				if params, active := cstorage[class]; active {
					entry.storageClasses[class] = true
					if params.isDefault {
						entry.storageClasses["default"] = true
					}
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
			c.log.Error("invalid node requested while iterating pods", "node-name", nodeName)
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
		cpu:              nd.cpu.dup(),
		memory:           nd.memory,
		ephemeralStorage: nd.ephemeralStorage,
		volumesAttached:  nd.volumesAttached,
		volumesMounted:   nd.volumesMounted,
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
