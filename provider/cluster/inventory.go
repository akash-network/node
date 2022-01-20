package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ovrclk/akash/provider/cluster/operatorclients"
	ipoptypes "github.com/ovrclk/akash/provider/operator/ipoperator/types"
	"github.com/ovrclk/akash/provider/operator/waiter"
	"sync/atomic"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/boz/go-lifecycle"
	"github.com/tendermint/tendermint/libs/log"

	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	clusterUtil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/pubsub"
	atypes "github.com/ovrclk/akash/types/v1beta2"
	"github.com/ovrclk/akash/util/runner"

	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

var (
	// errReservationNotFound is the new error with message "not found"
	errReservationNotFound      = errors.New("reservation not found")
	errInventoryNotAvailableYet = errors.New("inventory status not available yet")
	errInventoryReservation     = errors.New("inventory error")
	errNoLeasedIPsAvailable     = fmt.Errorf("%w: no leased IPs available", errInventoryReservation)
	errInsufficientIPs          = fmt.Errorf("%w: insufficient number of IPs", errInventoryReservation)
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

	ipOperator operatorclients.IPOperatorClient

	waiter waiter.OperatorWaiter
}

func newInventoryService(
	config Config,
	log log.Logger,
	donech <-chan struct{},
	sub pubsub.Subscriber,
	client Client,
	ipOperatorClient operatorclients.IPOperatorClient,
	waiter waiter.OperatorWaiter,
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
		ipOperator:             ipOperatorClient,
		waiter:                 waiter,
	}

	reservations := make([]*reservation, 0, len(deployments))
	for _, d := range deployments {
		reservations = append(reservations, newReservation(d.LeaseID().OrderID(), d.ManifestGroup()))
	}

	ctx, _ := TieContextToChannel(context.Background(), donech)

	go is.lc.WatchChannel(donech)
	go is.run(ctx, reservations)

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

func (is *inventoryService) resourcesToCommit(rgroup atypes.ResourceGroup) atypes.ResourceGroup {
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
			Endpoints: resource.Resources.GetEndpoints(),
		}

		storage := make(atypes.Volumes, 0, len(resource.Resources.GetStorage()))

		for _, volume := range resource.Resources.GetStorage() {
			storage = append(storage, atypes.Storage{
				Name:       volume.Name,
				Quantity:   clusterUtil.ComputeCommittedResources(is.config.StorageCommitLevel, volume.GetQuantity()),
				Attributes: volume.GetAttributes(),
			})
		}

		runits.Storage = storage

		v := dtypes.Resource{
			Resources: runits,
			Count:     resource.Count,
			Price:     sdk.DecCoin{},
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

func (is *inventoryService) updateInventoryMetrics(metrics ctypes.InventoryMetrics) {
	clusterInventoryAllocateable.WithLabelValues("nodes").Set(float64(len(metrics.Nodes)))

	clusterInventoryAllocateable.WithLabelValues("cpu").Set(float64(metrics.TotalAllocatable.CPU) / 1000)
	clusterInventoryAllocateable.WithLabelValues("memory").Set(float64(metrics.TotalAllocatable.Memory))
	clusterInventoryAllocateable.WithLabelValues("storage-ephemeral").Set(float64(metrics.TotalAllocatable.StorageEphemeral))
	for class, val := range metrics.TotalAllocatable.Storage {
		clusterInventoryAllocateable.WithLabelValues(fmt.Sprintf("storage-%s", class)).Set(float64(val))
	}

	clusterInventoryAllocateable.WithLabelValues("endpoints").Set(float64(is.config.InventoryExternalPortQuantity))

	clusterInventoryAvailable.WithLabelValues("cpu").Set(float64(metrics.TotalAvailable.CPU) / 1000)
	clusterInventoryAvailable.WithLabelValues("memory").Set(float64(metrics.TotalAvailable.Memory))
	clusterInventoryAvailable.WithLabelValues("storage-ephemeral").Set(float64(metrics.TotalAvailable.StorageEphemeral))
	for class, val := range metrics.TotalAvailable.Storage {
		clusterInventoryAvailable.WithLabelValues(fmt.Sprintf("storage-%s", class)).Set(float64(val))
	}

	clusterInventoryAvailable.WithLabelValues("endpoints").Set(float64(is.availableExternalPorts))
}

func updateReservationMetrics(reservations []*reservation) {
	inventoryReservations.WithLabelValues("none", "quantity").Set(float64(len(reservations)))

	activeCPUTotal := 0.0
	activeMemoryTotal := 0.0
	activeStorageEphemeralTotal := 0.0
	activeEndpointsTotal := 0.0

	pendingCPUTotal := 0.0
	pendingMemoryTotal := 0.0
	pendingStorageEphemeralTotal := 0.0
	pendingEndpointsTotal := 0.0

	allocated := 0.0
	for _, reservation := range reservations {
		cpuTotal := &pendingCPUTotal
		memoryTotal := &pendingMemoryTotal
		// storageTotal := &pendingStorageTotal
		endpointsTotal := &pendingEndpointsTotal

		if reservation.allocated {
			allocated++
			cpuTotal = &activeCPUTotal
			memoryTotal = &activeMemoryTotal
			// storageTotal = &activeStorageTotal
			endpointsTotal = &activeEndpointsTotal
		}
		for _, resource := range reservation.Resources().GetResources() {
			*cpuTotal += float64(resource.Resources.GetCPU().GetUnits().Value() * uint64(resource.Count))
			*memoryTotal += float64(resource.Resources.GetMemory().Quantity.Value() * uint64(resource.Count))
			// *storageTotal += float64(resource.Resources.GetStorage().Quantity.Value() * uint64(resource.Count))
			*endpointsTotal += float64(len(resource.Resources.GetEndpoints()))
		}
	}

	inventoryReservations.WithLabelValues("none", "allocated").Set(allocated)

	inventoryReservations.WithLabelValues("active", "cpu").Set(activeCPUTotal)
	inventoryReservations.WithLabelValues("active", "memory").Set(activeMemoryTotal)
	inventoryReservations.WithLabelValues("active", "storage-ephemeral").Set(activeStorageEphemeralTotal)
	inventoryReservations.WithLabelValues("active", "endpoints").Set(activeEndpointsTotal)

	inventoryReservations.WithLabelValues("pending", "cpu").Set(pendingCPUTotal)
	inventoryReservations.WithLabelValues("pending", "memory").Set(pendingMemoryTotal)
	inventoryReservations.WithLabelValues("pending", "storage-ephemeral").Set(pendingStorageEphemeralTotal)
	inventoryReservations.WithLabelValues("pending", "endpoints").Set(pendingEndpointsTotal)
}

type inventoryServiceState struct {
	inventory    ctypes.Inventory
	reservations []*reservation
	ipAddrUsage  ipoptypes.IPAddressUsage
}

func countPendingIPs(state *inventoryServiceState) uint {
	pending := uint(0)
	for _, entry := range state.reservations {
		if !entry.ipsConfirmed {
			pending += entry.endpointQuantity
		}
	}

	return pending
}

func (is *inventoryService) handleRequest(req inventoryRequest, state *inventoryServiceState) {
	// convert the resources to the committed amount
	resourcesToCommit := is.resourcesToCommit(req.resources)
	// create new registration if capacity available
	reservation := newReservation(req.order, resourcesToCommit)

	is.log.Debug("reservation requested", "order", req.order, "resources", req.resources)

	if reservation.endpointQuantity != 0 {
		if is.ipOperator == nil {
			req.ch <- inventoryResponse{err: errNoLeasedIPsAvailable}
			return
		}
		numIPUnused := state.ipAddrUsage.Available - state.ipAddrUsage.InUse
		pending := countPendingIPs(state)
		if reservation.endpointQuantity > (numIPUnused - pending) {
			is.log.Info("insufficient number of IP addresses available", "order", req.order)
			req.ch <- inventoryResponse{err: fmt.Errorf("%w: unable to reserve %d", errInsufficientIPs, reservation.endpointQuantity)}
			return
		}

		is.log.Info("reservation used leased IPs", "used", reservation.endpointQuantity, "available", state.ipAddrUsage.Available, "in-use", state.ipAddrUsage.InUse, "pending", pending)
	} else {
		reservation.ipsConfirmed = true // No IPs, just mark it as confirmed implicitly
	}

	err := state.inventory.Adjust(reservation)
	if err != nil {
		is.log.Info("insufficient capacity for reservation", "order", req.order)
		inventoryRequestsCounter.WithLabelValues("reserve", "insufficient-capacity").Inc()
		req.ch <- inventoryResponse{err: err}
		return
	}

	// Add the reservation to the list
	state.reservations = append(state.reservations, reservation)
	req.ch <- inventoryResponse{value: reservation}
	inventoryRequestsCounter.WithLabelValues("reserve", "create").Inc()

}

func (is *inventoryService) run(ctx context.Context, reservationsArg []*reservation) {
	defer is.lc.ShutdownCompleted()
	defer is.sub.Close()

	state := &inventoryServiceState{
		inventory:    nil,
		reservations: reservationsArg,
	}
	is.log.Info("starting with existing reservations", "qty", len(state.reservations))

	// wait on the operators to be ready
	err := is.waiter.WaitForAll(ctx)
	if err != nil {
		is.lc.ShutdownInitiated(err)
		return
	}

	// Create a timer to trigger periodic inventory checks
	// Stop the timer immediately
	t := time.NewTimer(time.Hour)
	t.Stop()
	defer t.Stop()

	// Run an inventory check immediately.
	runch := is.runCheck(ctx, state)

	var fetchCount uint

	var reserveChLocal <-chan inventoryRequest

	resumeProcessingReservations := func() {
		reserveChLocal = is.reservech
	}

	updateInventory := func() {
		reserveChLocal = nil
		if runch == nil {
			runch = is.runCheck(ctx, state)
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
				for _, res := range state.reservations {
					if !res.OrderID().Equals(ev.LeaseID.OrderID()) {
						continue
					}
					if res.Resources().GetName() != ev.Group.Name {
						continue
					}

					allocatedPrev := res.allocated
					res.allocated = ev.Status == event.ClusterDeploymentDeployed
					updateInventory()

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
			is.handleRequest(req, state)

		case req := <-is.lookupch:
			// lookup registration
			for _, res := range state.reservations {
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

			for idx, res := range state.reservations {
				if !res.OrderID().Equals(req.order) {
					continue
				}

				is.log.Info("removing reservation", "order", res.OrderID())

				state.reservations = append(state.reservations[:idx], state.reservations[idx+1:]...)
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
			responseCh <- is.getStatus(state)
			inventoryRequestsCounter.WithLabelValues("status", "success").Inc()

		case <-t.C:
			// run cluster inventory check

			t.Stop()
			// Run an inventory check
			updateInventory()

		case res := <-runch:
			// inventory check returned
			runch = nil

			// Reset the inventory check timer, so this runs periodically
			t.Reset(is.config.InventoryResourcePollPeriod)

			if err := res.Error(); err != nil {
				is.log.Error("checking inventory", "err", err)
				break
			}

			select {
			case <-is.readych:
				break
			default:
				is.log.Debug("inventory ready")
				close(is.readych)
			}

			resultArray := res.Value().([]interface{})

			state.inventory = resultArray[0].(ctypes.Inventory)
			metrics := state.inventory.Metrics()

			is.updateInventoryMetrics(metrics)

			if fetchCount%is.config.InventoryResourceDebugFrequency == 0 {
				buf := &bytes.Buffer{}
				enc := json.NewEncoder(buf)
				err := enc.Encode(&metrics)
				if err == nil {
					is.log.Debug("cluster resources", "dump", buf.String())
				} else {
					is.log.Error("unable to dump cluster inventory", "error", err.Error())
				}
			}
			fetchCount++

			// readjust inventory accordingly with pending leases
			for _, r := range state.reservations {
				if !r.allocated {
					if err := state.inventory.Adjust(r); err != nil {
						is.log.Error("adjust inventory for pending reservation", "error", err.Error())
					}
				}
			}

			if is.ipOperator != nil {
				// Save IP address data
				state.ipAddrUsage = resultArray[1].(ipoptypes.IPAddressUsage)

				// Process confirmed IP addresses usage
				confirmed := resultArray[2].([]mtypes.OrderID)

				for _, confirmedOrderID := range confirmed {

					for i, entry := range state.reservations {
						if entry.order.Equals(confirmedOrderID) {
							state.reservations[i].ipsConfirmed = true
							is.log.Info("confirmed IP allocation", "orderID", confirmedOrderID)
							break
						}
					}
				}
			}

			resumeProcessingReservations()
		}

		updateReservationMetrics(state.reservations)
	}

	if runch != nil {
		<-runch
	}

	if is.ipOperator != nil {
		is.ipOperator.Stop()
	}
}

type confirmationItem struct {
	orderID          mtypes.OrderID
	expectedQuantity uint
}

func (is *inventoryService) runCheck(ctx context.Context, state *inventoryServiceState) <-chan runner.Result {
	// Look for unconfirmed IPs, these are IPs that have an deployment created
	// event and are marked allocated. But until the IP address operator has reported
	// that it has actually created the associated resources, we need to consider the total number of end
	// points as pending

	var confirm []confirmationItem

	if is.ipOperator != nil {
		for _, entry := range state.reservations {
			// Skip anything already confirmed or not allocated
			if entry.ipsConfirmed || !entry.allocated {
				continue
			}

			confirm = append(confirm, confirmationItem{
				orderID:          entry.OrderID(),
				expectedQuantity: entry.endpointQuantity,
			})
		}
	}

	state = nil // Don't access state past here, it isn't safe

	return runner.Do(func() runner.Result {
		inventoryResult, err := is.client.Inventory(ctx)

		if err != nil {
			return runner.NewResult(nil, err)
		}

		var ipResult ipoptypes.IPAddressUsage
		if is.ipOperator != nil {
			ipResult, err = is.ipOperator.GetIPAddressUsage(ctx)
			if err != nil {
				return runner.NewResult(nil, err)
			}
		}

		var confirmed []mtypes.OrderID

		for _, confirmItem := range confirm {
			status, err := is.ipOperator.GetIPAddressStatus(ctx, confirmItem.orderID)
			if err != nil {
				// This error is not really fatal, so don't bail on this entirely. The other results
				// retrieved in this code are still valid
				is.log.Error("failed checking IP address usage", "orderID", confirmItem.orderID, "error", err)
				break
			}

			numConfirmed := uint(len(status))
			if numConfirmed == confirmItem.expectedQuantity {
				confirmed = append(confirmed, confirmItem.orderID)
			}
		}

		result := []interface{}{
			inventoryResult,
			ipResult,
			confirmed,
		}

		return runner.NewResult(result, nil)
	})
}

func (is *inventoryService) getStatus(state *inventoryServiceState) ctypes.InventoryStatus {
	status := ctypes.InventoryStatus{}
	if state.inventory == nil {
		status.Error = errInventoryNotAvailableYet
		return status
	}

	for _, reservation := range state.reservations {
		total := ctypes.InventoryMetricTotal{
			Storage: make(map[string]int64),
		}

		for _, resources := range reservation.Resources().GetResources() {
			total.AddResources(resources)
		}

		if reservation.allocated {
			status.Active = append(status.Active, total)
		} else {
			status.Pending = append(status.Pending, total)
		}
	}

	for _, nd := range state.inventory.Metrics().Nodes {
		status.Available.Nodes = append(status.Available.Nodes, nd.Available)
	}

	for class, size := range state.inventory.Metrics().TotalAvailable.Storage {
		status.Available.Storage = append(status.Available.Storage, ctypes.InventoryStorageStatus{Class: class, Size: size})
	}
	return status
}

func reservationCountEndpoints(reservation *reservation) uint {
	var externalPortCount uint

	resources := reservation.Resources().GetResources()
	// Count the number of endpoints per resource. The number of instances does not affect
	// the number of ports
	for _, resource := range resources {
		for _, endpoint := range resource.Resources.Endpoints {
			if endpoint.Kind == atypes.Endpoint_RANDOM_PORT {
				externalPortCount++
			}
		}
	}

	return externalPortCount
}
