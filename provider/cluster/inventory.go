package cluster

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/boz/go-lifecycle"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	clusterUtil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/pubsub"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util/runner"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tendermint/tendermint/libs/log"
)

var (
	// errNotFound is the new error with message "not found"
	errReservationNotFound = errors.New("reservation not found")
	// ErrInsufficientCapacity is the new error when capacity is insufficient
	ErrInsufficientCapacity = errors.New("insufficient capacity")
)

var (
	inventoryRequestsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name:        "provider_inventory_requests",
		Help:        "",
		ConstLabels: nil,
	}, []string{"action", "result"})

	inventoryReservations = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "provider_inventory_reservations_total",
		Help: "",
	}, []string{"classification", "quantity"})

	clusterInventoryAllocateable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "provider_inventory_allocateable_total",
		Help: "",
	}, []string{"quantity"})

	clusterInventoryAvailable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "provider_inventory_available_total",
		Help: "",
	}, []string{"quantity"})
)

type inventoryService struct {
	config Config
	client Client
	sub    pubsub.Subscriber

	statusch         chan chan<- ctypes.InventoryStatus
	lookupch         chan inventoryRequest
	reservech        chan inventoryRequest
	unreservech      chan inventoryRequest
	reservationCount int64

	readych chan struct{}

	log log.Logger
	lc  lifecycle.Lifecycle

	availableExternalPorts uint
}

func newInventoryService(
	config Config,
	log log.Logger,
	donech <-chan struct{},
	sub pubsub.Subscriber,
	client Client,
	deployments []ctypes.Deployment,
) (*inventoryService, error) {

	sub, err := sub.Clone()
	if err != nil {
		return nil, err
	}

	is := &inventoryService{
		config:                 config,
		client:                 client,
		sub:                    sub,
		statusch:               make(chan chan<- ctypes.InventoryStatus),
		lookupch:               make(chan inventoryRequest),
		reservech:              make(chan inventoryRequest),
		unreservech:            make(chan inventoryRequest),
		readych:                make(chan struct{}),
		log:                    log.With("cmp", "inventory-service"),
		lc:                     lifecycle.New(),
		availableExternalPorts: config.InventoryExternalPortQuantity,
	}

	reservations := make([]*reservation, 0, len(deployments))
	for _, d := range deployments {
		reservations = append(reservations, newReservation(d.LeaseID().OrderID(), d.ManifestGroup()))
	}

	go is.lc.WatchChannel(donech)
	go is.run(reservations)

	return is, nil
}

func (is *inventoryService) done() <-chan struct{} {
	return is.lc.Done()
}

func (is *inventoryService) ready() <-chan struct{} {
	return is.readych
}

func (is *inventoryService) lookup(order mtypes.OrderID, resources atypes.ResourceGroup) (ctypes.Reservation, error) {
	ch := make(chan inventoryResponse, 1)
	req := inventoryRequest{
		order:     order,
		resources: resources,
		ch:        ch,
	}

	select {
	case is.lookupch <- req:
		response := <-ch
		return response.value, response.err
	case <-is.lc.ShuttingDown():
		return nil, ErrNotRunning
	}
}

func (is *inventoryService) reserve(order mtypes.OrderID, resources atypes.ResourceGroup) (ctypes.Reservation, error) {
	ch := make(chan inventoryResponse, 1)
	req := inventoryRequest{
		order:     order,
		resources: resources,
		ch:        ch,
	}

	select {
	case is.reservech <- req:
		response := <-ch
		if response.err == nil {
			cnt := atomic.AddInt64(&is.reservationCount, 1)
			is.log.Debug("reservation count", "cnt", cnt)
		}
		return response.value, response.err
	case <-is.lc.ShuttingDown():
		return nil, ErrNotRunning
	}
}

func (is *inventoryService) unreserve(order mtypes.OrderID) error { // nolint:golint,unparam
	ch := make(chan inventoryResponse, 1)
	req := inventoryRequest{
		order: order,
		ch:    ch,
	}

	select {
	case is.unreservech <- req:
		response := <-ch
		if response.err == nil {
			cnt := atomic.AddInt64(&is.reservationCount, -1)
			is.log.Debug("reservation count", "cnt", cnt)
		}
		return response.err
	case <-is.lc.ShuttingDown():
		return ErrNotRunning
	}
}

func (is *inventoryService) status(ctx context.Context) (ctypes.InventoryStatus, error) {
	ch := make(chan ctypes.InventoryStatus, 1)

	select {
	case <-is.lc.Done():
		return ctypes.InventoryStatus{}, ErrNotRunning
	case <-ctx.Done():
		return ctypes.InventoryStatus{}, ctx.Err()
	case is.statusch <- ch:
	}

	select {
	case <-is.lc.Done():
		return ctypes.InventoryStatus{}, ErrNotRunning
	case <-ctx.Done():
		return ctypes.InventoryStatus{}, ctx.Err()
	case result := <-ch:
		return result, nil
	}
}

type inventoryRequest struct {
	order     mtypes.OrderID
	resources atypes.ResourceGroup
	ch        chan<- inventoryResponse
}

type inventoryResponse struct {
	value ctypes.Reservation
	err   error
}

func (is *inventoryService) committedResources(rgroup atypes.ResourceGroup) atypes.ResourceGroup {
	replacedResources := make([]dtypes.Resource, 0)

	for _, resource := range rgroup.GetResources() {
		runits := atypes.ResourceUnits{
			CPU: &atypes.CPU{
				Units:      clusterUtil.ComputeCommittedResources(is.config.CPUCommitLevel, resource.Resources.GetCPU().GetUnits()),
				Attributes: resource.Resources.GetCPU().GetAttributes(),
			},
			Memory: &atypes.Memory{
				Quantity:   clusterUtil.ComputeCommittedResources(is.config.MemoryCommitLevel, resource.Resources.GetMemory().GetQuantity()),
				Attributes: resource.Resources.GetMemory().GetAttributes(),
			},
			Storage: &atypes.Storage{
				Quantity:   clusterUtil.ComputeCommittedResources(is.config.StorageCommitLevel, resource.Resources.GetStorage().GetQuantity()),
				Attributes: resource.Resources.GetStorage().GetAttributes(),
			},
			Endpoints: resource.Resources.GetEndpoints(),
		}
		v := dtypes.Resource{
			Resources: runits,
			Count:     resource.Count,
			Price:     sdk.Coin{},
		}

		replacedResources = append(replacedResources, v)
	}

	result := dtypes.GroupSpec{
		Name:         rgroup.GetName(),
		Requirements: atypes.PlacementRequirements{},
		Resources:    replacedResources,
	}
	return result
}

func (is *inventoryService) updateInventoryMetrics(inventory []ctypes.Node) {
	clusterInventoryAllocateable.WithLabelValues("nodes").Set(float64(len(inventory)))

	cpuTotal := 0.0
	memoryTotal := 0.0
	storageTotal := 0.0

	cpuAvailable := 0.0
	memoryAvailable := 0.0
	storageAvailable := 0.0

	for _, node := range inventory {
		tmp := node.Allocateable()
		cpuTotal += float64((&tmp).GetCPU().GetUnits().Value())
		memoryTotal += float64((&tmp).GetMemory().Quantity.Value())
		storageTotal += float64((&tmp).GetStorage().Quantity.Value())

		tmp = node.Available()
		cpuAvailable += float64((&tmp).GetCPU().GetUnits().Value())
		memoryAvailable += float64((&tmp).GetMemory().Quantity.Value())
		storageAvailable += float64((&tmp).GetStorage().Quantity.Value())
	}

	clusterInventoryAllocateable.WithLabelValues("cpu").Set(cpuTotal)
	clusterInventoryAllocateable.WithLabelValues("memory").Set(memoryTotal)
	clusterInventoryAllocateable.WithLabelValues("storage").Set(storageTotal)
	clusterInventoryAllocateable.WithLabelValues("endpoints").Set(float64(is.config.InventoryExternalPortQuantity))

	clusterInventoryAvailable.WithLabelValues("cpu").Set(cpuAvailable)
	clusterInventoryAvailable.WithLabelValues("memory").Set(memoryAvailable)
	clusterInventoryAvailable.WithLabelValues("storage").Set(storageAvailable)
	clusterInventoryAvailable.WithLabelValues("endpoints").Set(float64(is.availableExternalPorts))
}

func updateReservationMetrics(reservations []*reservation) {
	inventoryReservations.WithLabelValues("none", "quantity").Set(float64(len(reservations)))

	activeCPUTotal := 0.0
	activeMemoryTotal := 0.0
	activeStorageTotal := 0.0
	activeEndpointsTotal := 0.0

	pendingCPUTotal := 0.0
	pendingMemoryTotal := 0.0
	pendingStorageTotal := 0.0
	pendingEndpointsTotal := 0.0

	allocated := 0.0
	for _, reservation := range reservations {
		cpuTotal := &pendingCPUTotal
		memoryTotal := &pendingMemoryTotal
		storageTotal := &pendingStorageTotal
		endpointsTotal := &pendingEndpointsTotal

		if reservation.allocated {
			allocated++
			cpuTotal = &activeCPUTotal
			memoryTotal = &activeMemoryTotal
			storageTotal = &activeStorageTotal
			endpointsTotal = &activeEndpointsTotal
		}
		for _, resource := range reservation.Resources().GetResources() {
			*cpuTotal += float64(resource.Resources.GetCPU().GetUnits().Value() * uint64(resource.Count))
			*memoryTotal += float64(resource.Resources.GetMemory().Quantity.Value() * uint64(resource.Count))
			*storageTotal += float64(resource.Resources.GetStorage().Quantity.Value() * uint64(resource.Count))
			*endpointsTotal += float64(len(resource.Resources.GetEndpoints()))
		}
	}

	inventoryReservations.WithLabelValues("none", "allocated").Set(allocated)

	inventoryReservations.WithLabelValues("active", "cpu").Set(activeCPUTotal)
	inventoryReservations.WithLabelValues("active", "memory").Set(activeMemoryTotal)
	inventoryReservations.WithLabelValues("active", "storage").Set(activeStorageTotal)
	inventoryReservations.WithLabelValues("active", "endpoints").Set(activeEndpointsTotal)

	inventoryReservations.WithLabelValues("pending", "cpu").Set(pendingCPUTotal)
	inventoryReservations.WithLabelValues("pending", "memory").Set(pendingMemoryTotal)
	inventoryReservations.WithLabelValues("pending", "storage").Set(pendingStorageTotal)
	inventoryReservations.WithLabelValues("pending", "endpoints").Set(pendingEndpointsTotal)
}

func (is *inventoryService) run(reservations []*reservation) {
	defer is.lc.ShutdownCompleted()
	defer is.sub.Close()
	ctx, cancel := context.WithCancel(context.Background())

	// Create a timer to trigger periodic inventory checks
	// Stop the timer immediately
	t := time.NewTimer(time.Hour)
	t.Stop()
	defer t.Stop()

	var inventory []ctypes.Node
	ready := false

	// Run an inventory check immediately.
	runch := is.runCheck(ctx)

	var fetchCount uint

	var reserveChLocal <-chan inventoryRequest
	allowProcessingReservations := func() {
		reserveChLocal = is.reservech
	}

	stopProcessingReservations := func() {
		reserveChLocal = nil
		if runch == nil {
			runch = is.runCheck(ctx)
		}
	}

loop:
	for {
		select {
		case err := <-is.lc.ShutdownRequest():
			is.lc.ShutdownInitiated(err)
			break loop

		case ev := <-is.sub.Events():
			switch ev := ev.(type) { // nolint: gocritic
			case event.ClusterDeployment:
				// mark reservation allocated if deployment successful
				for _, res := range reservations {
					if !res.OrderID().Equals(ev.LeaseID.OrderID()) {
						continue
					}
					if res.Resources().GetName() != ev.Group.Name {
						continue
					}

					allocatedPrev := res.allocated
					res.allocated = ev.Status == event.ClusterDeploymentDeployed
					stopProcessingReservations()

					if res.allocated != allocatedPrev {
						externalPortCount := reservationCountEndpoints(res)
						if ev.Status == event.ClusterDeploymentDeployed {
							is.availableExternalPorts -= externalPortCount
						} else {
							is.availableExternalPorts += externalPortCount
						}
					}

					is.log.Debug("reservation status update",
						"order", res.OrderID(),
						"resource-group", res.Resources().GetName(),
						"allocated", res.allocated)

					break
				}
			}

		case req := <-reserveChLocal:
			// convert the resources to the commmitted amount
			resourcesToCommit := is.committedResources(req.resources)
			// create new registration if capacity available
			reservation := newReservation(req.order, resourcesToCommit)

			is.log.Debug("reservation requested", "order", req.order, "resources", req.resources)

			if reservationAllocateable(inventory, is.availableExternalPorts, reservations, reservation) {
				reservations = append(reservations, reservation)
				req.ch <- inventoryResponse{value: reservation}
				inventoryRequestsCounter.WithLabelValues("reserve", "create").Inc()
				break
			}

			is.log.Info("insufficient capacity for reservation", "order", req.order)
			inventoryRequestsCounter.WithLabelValues("reserve", "insufficient-capacity").Inc()
			req.ch <- inventoryResponse{err: ErrInsufficientCapacity}

		case req := <-is.lookupch:
			// lookup registration

			for _, res := range reservations {
				if !res.OrderID().Equals(req.order) {
					continue
				}
				if res.Resources().GetName() != req.resources.GetName() {
					continue
				}
				req.ch <- inventoryResponse{value: res}
				inventoryRequestsCounter.WithLabelValues("lookup", "found").Inc()
				continue loop
			}

			inventoryRequestsCounter.WithLabelValues("lookup", "not-found").Inc()
			req.ch <- inventoryResponse{err: errReservationNotFound}

		case req := <-is.unreservech:
			is.log.Debug("unreserving capacity", "order", req.order)
			// remove reservation

			is.log.Info("attempting to removing reservation", "order", req.order)

			for idx, res := range reservations {
				if !res.OrderID().Equals(req.order) {
					continue
				}

				is.log.Info("removing reservation", "order", res.OrderID())

				reservations = append(reservations[:idx], reservations[idx+1:]...)
				// reclaim availableExternalPorts if unreserving allocated resources
				if res.allocated {
					is.availableExternalPorts += reservationCountEndpoints(res)
				}

				req.ch <- inventoryResponse{value: res}
				is.log.Info("unreserve capacity complete", "order", req.order)
				inventoryRequestsCounter.WithLabelValues("unreserve", "destroyed").Inc()
				continue loop
			}

			inventoryRequestsCounter.WithLabelValues("unreserve", "not-found").Inc()
			req.ch <- inventoryResponse{err: errReservationNotFound}

		case responseCh := <-is.statusch:
			responseCh <- is.getStatus(inventory, reservations)
			inventoryRequestsCounter.WithLabelValues("status", "success").Inc()

		case <-t.C:
			// run cluster inventory check

			t.Stop()
			// Run an inventory check
			runch = is.runCheck(ctx)

		case res := <-runch:
			// inventory check returned

			runch = nil

			// Reset the inventory check timer, so this runs periodically
			t.Reset(is.config.InventoryResourcePollPeriod)

			if err := res.Error(); err != nil {
				is.log.Error("checking inventory", "err", err)
				break
			}

			if !ready {
				is.log.Debug("inventory ready")
				ready = true
				close(is.readych)
			}

			inventory = res.Value().([]ctypes.Node)
			is.updateInventoryMetrics(inventory)
			if fetchCount%is.config.InventoryResourceDebugFrequency == 0 {
				is.log.Debug("inventory fetched", "nodes", len(inventory))
				for _, node := range inventory {
					available := node.Available()
					is.log.Debug("node resources",
						"node-id", node.ID(),
						"available-cpu", available.CPU,
						"available-memory", available.Memory,
						"available-storage", available.Storage)
				}
			}
			fetchCount++
			allowProcessingReservations()
		}
		updateReservationMetrics(reservations)
	}
	cancel()

	if runch != nil {
		<-runch
	}
}

func (is *inventoryService) runCheck(ctx context.Context) <-chan runner.Result {
	return runner.Do(func() runner.Result {
		return runner.NewResult(is.client.Inventory(ctx))
	})
}

func (is *inventoryService) getStatus(inventory []ctypes.Node, reservations []*reservation) ctypes.InventoryStatus {
	status := ctypes.InventoryStatus{}
	for _, reserve := range reservations {
		total := atypes.ResourceUnits{}

		for _, resource := range reserve.Resources().GetResources() {
			// 🤔
			if total, status.Error = total.Add(resource.Resources); status.Error != nil {
				return status
			}
		}

		if reserve.allocated {
			status.Active = append(status.Active, total)
		} else {
			status.Pending = append(status.Pending, total)
		}
	}

	for _, node := range inventory {
		status.Available = append(status.Available, node.Available())
	}

	return status
}

func reservationAllocateable(inventory []ctypes.Node, externalPortsAvailable uint, reservations []*reservation, newReservation *reservation) bool {
	// 1. for each unallocated reservation, subtract its resources
	//    from inventory.
	// 2. subtract resources for new reservation from inventory.
	// 3. return true iff 1 and 2 succeed.

	var ok bool

	for _, res := range reservations {
		if res.allocated {
			continue
		}
		inventory, externalPortsAvailable, ok = reservationAdjustInventory(inventory, externalPortsAvailable, res)
		if !ok {
			return false
		}
	}

	_, _, ok = reservationAdjustInventory(inventory, externalPortsAvailable, newReservation)

	return ok
}

func reservationCountEndpoints(reservation *reservation) uint {
	var externalPortCount uint

	resources := reservation.Resources().GetResources()
	// Count the number of endpoints per resource. The number of instances does not affect
	// the number of ports
	for _, resource := range resources {
		externalPortCount += uint(len(resource.Resources.Endpoints))
	}

	return externalPortCount
}

func reservationAdjustInventory(prevInventory []ctypes.Node, externalPortsAvailable uint, reservation *reservation) ([]ctypes.Node, uint, bool) {
	// for each node in the inventory
	//   subtract resource capacity from node capacity if the former will fit in the latter
	//   remove resource capacity that fit in node capacity from requested resource capacity
	// return remaining inventory, true iff all resources are able to fit

	resources := make([]atypes.Resources, len(reservation.resources.GetResources()))
	copy(resources, reservation.resources.GetResources())

	inventory := make([]ctypes.Node, 0, len(prevInventory))

	externalPortCount := reservationCountEndpoints(reservation)
	if externalPortsAvailable < externalPortCount {
		return nil, 0, false
	}
	externalPortsAvailable -= externalPortCount
	for _, node := range prevInventory {
		available := node.Available()
		curResources := resources[:0]

		for _, resource := range resources {
			for ; resource.Count > 0; resource.Count-- {
				var err error
				var remaining atypes.ResourceUnits
				if remaining, err = available.Sub(resource.Resources); err != nil {
					break
				}
				available = remaining
			}

			if resource.Count > 0 {
				curResources = append(curResources, resource)
			}
		}

		resources = curResources
		inventory = append(inventory, NewNode(node.ID(), node.Allocateable(), available))
	}

	return inventory, externalPortsAvailable, len(resources) == 0
}
