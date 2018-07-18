package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
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

func NewService(ctx context.Context, session session.Session, bus event.Bus, client Client) (Service, error) {

	log := session.Log().With("module", "provider-cluster")

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	deployments, err := client.Deployments()
	if err != nil {
		log.Error("fetching deployments", "err", err)
		sub.Close()
		return nil, err
	}
	log.Info("found managed deployments", "count", len(deployments))

	s := &service{
		session:     session,
		client:      client,
		bus:         bus,
		sub:         sub,
		deployments: make(map[string]*managedDeployment),
		managerch:   make(chan *deploymentManager),
		reservech:   make(chan reserveRequest),
		log:         log,
		lc:          lifecycle.New(),
	}

	go s.lc.WatchContext(ctx)
	go s.run(deployments)

	return s, nil
}

type service struct {
	session session.Session
	client  Client
	bus     event.Bus
	sub     event.Subscriber

	deployments map[string]*managedDeployment

	reservech chan reserveRequest
	managerch chan *deploymentManager

	log log.Logger
	lc  lifecycle.Lifecycle
}

type managedDeployment struct {
	reservation Reservation
	manager     *deploymentManager
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

func (s *service) run(deployments []Deployment) {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

	for _, deployment := range deployments {
		// TODO: recover reservation
		key := deployment.LeaseID().OrderID().String()
		s.deployments[key] = &managedDeployment{
			manager:     newDeploymentManager(s, deployment.LeaseID(), deployment.ManifestGroup()),
			reservation: newReservation(deployment.LeaseID().OrderID(), nil),
		}
	}

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

				if state.manager == nil {
					state.manager = newDeploymentManager(s, ev.LeaseID, mgroup)
					break
				}

				if err := state.manager.update(mgroup); err != nil {
					s.log.Error("updating deployment", "err", err, "lease", ev.LeaseID)
				}

			case *event.TxCloseDeployment:

				// teardown/undeploy managed deployments

				for _, dm := range s.deployments {
					if bytes.Equal(ev.Deployment, dm.reservation.OrderID().DeploymentID()) {
						s.teardownOrder(dm.reservation.OrderID())
					}
				}

			case *event.TxCloseFulfillment:

				s.teardownOrder(ev.OrderID())

			case *event.TxCloseLease:

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

		case dm := <-s.managerch:

			s.log.Debug("manager done", "order", dm.lease.OrderID())

			// todo: unreserve resources

			delete(s.deployments, dm.lease.OrderID().String())
		}
	}

	s.log.Debug("draining deployment managers...")

	for _, state := range s.deployments {
		if state.manager != nil {
			dm := <-s.managerch
			s.log.Debug("manager done", "order", dm.lease.OrderID())
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

	if state.manager == nil {
		delete(s.deployments, key)
		return
	}

	if err := state.manager.teardown(); err != nil {
		s.log.Error("tearing down deployment", "err", err, "order", oid)
	}
}
