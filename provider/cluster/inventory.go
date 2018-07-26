package cluster

import (
	"errors"
	"time"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util/runner"
	"github.com/tendermint/tmlibs/log"
)

const (
	inventoryResourcePollPeriod = time.Second
)

var (
	errNotFound             = errors.New("not found")
	ErrInsufficientCapacity = errors.New("insufficient capacity")
)

type inventoryService struct {
	client Client
	sub    event.Subscriber

	lookupch    chan inventoryRequest
	reservech   chan inventoryRequest
	unreservech chan inventoryRequest

	readych chan struct{}

	log log.Logger
	lc  lifecycle.Lifecycle
}

func newInventoryService(log log.Logger,
	donech <-chan struct{},
	sub event.Subscriber,
	client Client,
	deployments []Deployment,
) (*inventoryService, error) {

	sub, err := sub.Clone()
	if err != nil {
		return nil, err
	}

	is := &inventoryService{
		client:      client,
		sub:         sub,
		lookupch:    make(chan inventoryRequest),
		reservech:   make(chan inventoryRequest),
		unreservech: make(chan inventoryRequest),
		readych:     make(chan struct{}),
		log:         log.With("cmp", "inventory-service"),
		lc:          lifecycle.New(),
	}

	reservations := make([]*reservation, 0, len(deployments))
	for _, d := range deployments {
		reservations = append(reservations,
			newReservation(d.LeaseID().OrderID(), d.ManifestGroup()))
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

func (is *inventoryService) lookup(order types.OrderID, resources types.ResourceList) (Reservation, error) {
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

func (is *inventoryService) reserve(order types.OrderID, resources types.ResourceList) (Reservation, error) {
	ch := make(chan inventoryResponse, 1)
	req := inventoryRequest{
		order:     order,
		resources: resources,
		ch:        ch,
	}

	select {
	case is.reservech <- req:
		response := <-ch
		return response.value, response.err
	case <-is.lc.ShuttingDown():
		return nil, ErrNotRunning
	}
}

func (is *inventoryService) unreserve(order types.OrderID, resources types.ResourceList) (Reservation, error) {
	ch := make(chan inventoryResponse, 1)
	req := inventoryRequest{
		order:     order,
		resources: resources,
		ch:        ch,
	}

	select {
	case is.unreservech <- req:
		response := <-ch
		return response.value, response.err
	case <-is.lc.ShuttingDown():
		return nil, ErrNotRunning
	}
}

type inventoryRequest struct {
	order     types.OrderID
	resources types.ResourceList
	ch        chan<- inventoryResponse
}

type inventoryResponse struct {
	value Reservation
	err   error
}

func (is *inventoryService) run(reservations []*reservation) {
	defer is.lc.ShutdownCompleted()
	defer is.sub.Close()

	t := time.NewTimer(time.Hour)
	t.Stop()
	defer t.Stop()

	var inventory []Node
	ready := false
	runch := is.runCheck()

loop:
	for {
		select {
		case err := <-is.lc.ShutdownRequest():
			is.lc.ShutdownInitiated(err)
			break loop

		case ev := <-is.sub.Events():

			switch ev := ev.(type) {
			case event.ClusterDeployment:
				// mark reservation allocated if deployment successful

				for _, res := range reservations {
					if res.OrderID().Compare(ev.LeaseID.OrderID()) != 0 {
						continue
					}
					if res.Resources().GetName() != ev.Group.Name {
						continue
					}

					res.allocated = ev.Status == event.ClusterDeploymentDeployed

					is.log.Debug("reservation status update",
						"order", res.OrderID(),
						"resource-group", res.Resources().GetName(),
						"allocated", res.allocated)

					break
				}
			}

		case req := <-is.reservech:
			// create new registration if capacity available

			reservation := newReservation(req.order, req.resources)

			is.log.Debug("reservation requested", "order", req.order, "resources", req.resources)

			if reservationAllocateable(inventory, reservations, reservation) {
				reservations = append(reservations, reservation)
				req.ch <- inventoryResponse{value: reservation}
				break
			}

			is.log.Info("insufficient capacity for reservation", "order", req.order)

			req.ch <- inventoryResponse{err: ErrInsufficientCapacity}

		case req := <-is.lookupch:
			// lookup registration

			for _, res := range reservations {
				if res.OrderID().Compare(req.order) != 0 {
					continue
				}
				if res.Resources().GetName() != req.resources.GetName() {
					continue
				}
				req.ch <- inventoryResponse{value: res}
				continue loop
			}

			req.ch <- inventoryResponse{err: errNotFound}

		case req := <-is.unreservech:
			// remove reservation

			for idx, res := range reservations {
				if res.OrderID().Compare(req.order) != 0 {
					continue
				}
				if res.Resources().GetName() != req.resources.GetName() {
					continue
				}

				reservations = append(reservations[:idx], reservations[idx+1:]...)

				req.ch <- inventoryResponse{value: res}
				continue loop
			}

			req.ch <- inventoryResponse{err: errNotFound}

		case <-t.C:
			// run cluster inventory check

			t.Stop()
			runch = is.runCheck()

		case res := <-runch:
			// inventory check returned

			runch = nil
			t.Reset(inventoryResourcePollPeriod)

			if err := res.Error(); err != nil {
				is.log.Error("checking inventory", "err", err)
				break
			}

			if !ready {
				is.log.Debug("inventory ready")
				ready = true
				close(is.readych)
			}

			inventory = res.Value().([]Node)

			is.log.Debug("inventory fetched", "nodes", len(inventory))

			for _, node := range inventory {
				available := node.Available()
				is.log.Debug("node resources",
					"node-id", node.ID(),
					"available-cpu", available.CPU,
					"available-memory", available.Memory,
					"available-disk", available.Disk)
			}
		}
	}

	if runch != nil {
		<-runch
	}
}

func (is *inventoryService) runCheck() <-chan runner.Result {
	return runner.Do(func() runner.Result {
		return runner.NewResult(is.client.Inventory())
	})
}

func reservationAllocateable(inventory []Node, reservations []*reservation, newReservation *reservation) bool {

	// 1. for each unallocated reservation, subtract its resources
	//    from inventory.
	// 2. subtract resources for new reservation from inventory.
	// 3. return true iff 1 and 2 succeed.

	var ok bool

	for _, res := range reservations {
		if res.allocated {
			continue
		}
		inventory, ok = reservationAdjustInventory(inventory, res)
		if !ok {
			return false
		}
	}

	_, ok = reservationAdjustInventory(inventory, newReservation)

	return ok
}

func reservationAdjustInventory(prevInventory []Node, reservation *reservation) ([]Node, bool) {

	// for each node in the inventory
	//   subtract resource capacity from node capacity if the former will fit in the latter
	//   remove resource capacity that fit in node capacity from requested resource capacity
	// return remaining inventory, true iff all resources were able to fit

	resources := make([]types.ResourceGroup, len(reservation.resources.GetResources()))
	copy(resources, reservation.resources.GetResources())

	inventory := make([]Node, 0, len(prevInventory))

	for _, node := range prevInventory {
		available := node.Available()

		curResources := resources[:0]

		for _, resource := range resources {

			for ; resource.Count > 0; resource.Count-- {
				if available.CPU < resource.Unit.CPU ||
					available.Memory < resource.Unit.Memory ||
					available.Disk < resource.Unit.Disk {
					break
				}
				available.CPU -= resource.Unit.CPU
				available.Memory -= resource.Unit.Memory
				available.Disk -= resource.Unit.Disk
			}

			if resource.Count > 0 {
				curResources = append(curResources, resource)
			}
		}

		resources = curResources
		inventory = append(inventory, NewNode(node.ID(), available))
	}

	return inventory, len(resources) == 0
}
