package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	dtypes "github.com/ovrclk/akash/x/deployment/types"

	"github.com/boz/go-lifecycle"
	"github.com/tendermint/tendermint/libs/log"

	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	clusterUtil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/pubsub"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util/runner"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

var (
	// errReservationNotFound is the new error with message "not found"
	errReservationNotFound      = errors.New("reservation not found")
	errInventoryNotAvailableYet = errors.New("inventory status not available yet")
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

func (is *inventoryService) run(reservations []*reservation) {
	defer is.lc.ShutdownCompleted()
	defer is.sub.Close()
	ctx, cancel := context.WithCancel(context.Background())

	// Create a timer to trigger periodic inventory checks
	// Stop the timer immediately
	t := time.NewTimer(time.Hour)
	t.Stop()
	defer t.Stop()

	var inventory ctypes.Inventory

	// Run an inventory check immediately.
	runch := is.runCheck(ctx)

	var fetchCount uint

	var reserveChLocal <-chan inventoryRequest

	resumeProcessingReservations := func() {
		reserveChLocal = is.reservech
	}

	updateInventory := func() {
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
			// convert the resources to the committed amount
			resourcesToCommit := is.resourcesToCommit(req.resources)
			// create new registration if capacity available
			reservation := newReservation(req.order, resourcesToCommit)

			is.log.Debug("reservation requested", "order", req.order, "resources", req.resources)

			err := inventory.Adjust(reservation)
			if err == nil {
				reservations = append(reservations, reservation)
				req.ch <- inventoryResponse{value: reservation}
				inventoryRequestsCounter.WithLabelValues("reserve", "create").Inc()
				break
			}

			is.log.Info("insufficient capacity for reservation", "order", req.order)
			inventoryRequestsCounter.WithLabelValues("reserve", "insufficient-capacity").Inc()
			req.ch <- inventoryResponse{err: err}

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

			select {
			case _ = <-is.readych:
				break
			default:
				is.log.Debug("inventory ready")
				close(is.readych)
			}

			inventory = res.Value().(ctypes.Inventory)
			metrics := inventory.Metrics()

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
			for _, r := range reservations {
				if !r.allocated {
					if err := inventory.Adjust(r); err != nil {
						is.log.Error("adjust inventory for pending reservation", "error", err.Error())
					}
				}
			}

			resumeProcessingReservations()
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

func (is *inventoryService) getStatus(inventory ctypes.Inventory, reservations []*reservation) ctypes.InventoryStatus {
	status := ctypes.InventoryStatus{}
	if inventory == nil {
		status.Error = errInventoryNotAvailableYet
		return status
	}

	for _, reservation := range reservations {
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

	for _, nd := range inventory.Metrics().Nodes {
		status.Available.Nodes = append(status.Available.Nodes, nd.Available)
	}

	for class, size := range inventory.Metrics().TotalAvailable.Storage {
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
		externalPortCount += uint(len(resource.Resources.Endpoints))
	}

	return externalPortCount
}
