package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
)

var ErrNotRunning = errors.New("not running")

type Cluster interface {
	Reserve(types.OrderID, *types.DeploymentGroup) (Reservation, error)
}

// Manage compute cluster for the provider.  Will eventually integrate with kubernetes, etc...
type Service interface {
	Cluster
	Close() error
	Done() <-chan struct{}
}

func NewService(log log.Logger, ctx context.Context, bus event.Bus, client Client) (Service, error) {

	log = log.With("module", "provider-cluster")

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	s := &service{
		client:      client,
		bus:         bus,
		sub:         sub,
		deployments: make(map[string]*managedDeployment),
		monitorch:   make(chan *deploymentMonitor),
		reservech:   make(chan reserveRequest),
		log:         log,
		lc:          lifecycle.New(),
	}

	go s.lc.WatchContext(ctx)
	go s.run()

	return s, nil
}

type service struct {
	client Client
	bus    event.Bus
	sub    event.Subscriber

	deployments map[string]*managedDeployment

	reservech chan reserveRequest
	monitorch chan *deploymentMonitor

	log log.Logger
	lc  lifecycle.Lifecycle
}

type managedDeployment struct {
	reservation Reservation
	monitor     *deploymentMonitor
}

func (s *service) Close() error {
	s.lc.Shutdown(nil)
	return s.lc.Error()
}

func (s *service) Done() <-chan struct{} {
	return s.lc.Done()
}

func (s *service) Reserve(order types.OrderID, group *types.DeploymentGroup) (Reservation, error) {
	ch := make(chan reserveResponse, 1)
	req := reserveRequest{
		order: order,
		group: group,
		ch:    ch,
	}

	select {
	case s.reservech <- req:
		response := <-ch
		return response.value, response.err
	case <-s.lc.ShuttingDown():
		return nil, ErrNotRunning
	}
}

type reserveRequest struct {
	order types.OrderID
	group *types.DeploymentGroup
	ch    chan<- reserveResponse
}

type reserveResponse struct {
	value Reservation
	err   error
}

func (s *service) run() {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

loop:
	for {
		select {
		case err := <-s.lc.ShutdownRequest():
			s.lc.ShutdownInitiated(err)
			break loop

		case ev := <-s.sub.Events():
			switch ev := ev.(type) {
			case event.ManifestReceived:
				s.log.Info("manifest received")

				mgroup := ev.ManifestGroup()
				if mgroup == nil {
					s.log.Error("indeterminate manifest group", "lease", ev.LeaseID, "group-name", ev.Group.Name)
					break
				}

				key := ev.LeaseID.OrderID().String()

				state := s.deployments[key]

				if state == nil {
					s.log.Error("lease received without reservation")
					break
				}

				if state.monitor == nil {
					state.monitor = newDeploymentMonitor(s, ev.LeaseID, ev.Group, mgroup)
					break
				}

				if err := state.monitor.update(mgroup); err != nil {
					s.log.Error("updating deployment", "err", err)
				}

			case *event.TxCloseDeployment:

				// teardown/undeploy managed deployments

				for _, dm := range s.deployments {
					if bytes.Compare(ev.Deployment, dm.reservation.OrderID().DeploymentID()) == 0 {
						s.teardownOrder(dm.reservation.OrderID())
					}
				}

			case *event.TxCloseFulfillment:

				s.teardownOrder(ev.OrderID())

			}
		case req := <-s.reservech:
			// TODO: handle inventory

			key := req.order.String()

			state := s.deployments[key]
			if state != nil {
				s.log.Error("reservation exists", "order", req.order)
				req.ch <- reserveResponse{nil, fmt.Errorf("reservation exists")}
				break
			}

			state = &managedDeployment{
				reservation: newReservation(req.order, req.group),
			}

			s.deployments[key] = state

			req.ch <- reserveResponse{state.reservation, nil}

		case dm := <-s.monitorch:

			s.log.Debug("monitor done", "order", dm.lease.OrderID())

			// todo: unreserve resources

			delete(s.deployments, dm.lease.OrderID().String())
		}
	}

	s.log.Debug("draining deployment monitors...")

	for _, state := range s.deployments {
		if state.monitor != nil {
			dm := <-s.monitorch
			s.log.Debug("monitor done", "order", dm.lease.OrderID())
		}
	}

}

func (s *service) teardownOrder(oid types.OrderID) {
	key := oid.String()
	state := s.deployments[key]
	if state == nil {
		return
	}

	s.log.Debug("unregistering order", "order", oid)

	if state.monitor == nil {
		delete(s.deployments, key)
		return
	}

	if err := state.monitor.teardown(); err != nil {
		s.log.Error("tearing down deployment", "err", err, "order", oid)
	}
}
