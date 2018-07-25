package cluster

import (
	"bytes"
	"context"
	"errors"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
)

var ErrNotRunning = errors.New("not running")

type Cluster interface {
	Reserve(types.OrderID, *types.DeploymentGroup) (Reservation, error)
	Unreserve(types.OrderID, types.ResourceList) error
}

// Manage compute cluster for the provider.  Will eventually integrate with kubernetes, etc...
type Service interface {
	Cluster
	Close() error
	Done() <-chan struct{}
}

func NewService(ctx context.Context, session session.Session, bus event.Bus, client Client) (Service, error) {

	log := session.Log().With("module", "provider-cluster", "cmp", "service")

	lc := lifecycle.New()

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

	inventory, err := newInventoryService(log, lc.ShuttingDown(), sub, client, deployments)
	if err != nil {
		sub.Close()
		return nil, err
	}

	s := &service{
		session:   session,
		client:    client,
		bus:       bus,
		sub:       sub,
		inventory: inventory,
		managers:  make(map[string]*deploymentManager),
		managerch: make(chan *deploymentManager),
		log:       log,
		lc:        lc,
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

	inventory *inventoryService

	managers  map[string]*deploymentManager
	managerch chan *deploymentManager

	log log.Logger
	lc  lifecycle.Lifecycle
}

func (s *service) Close() error {
	s.lc.Shutdown(nil)
	return s.lc.Error()
}

func (s *service) Done() <-chan struct{} {
	return s.lc.Done()
}

func (s *service) Reserve(order types.OrderID, group *types.DeploymentGroup) (Reservation, error) {
	return s.inventory.reserve(order, group)
}

func (s *service) Unreserve(order types.OrderID, resources types.ResourceList) error {
	_, err := s.inventory.unreserve(order, resources)
	return err
}

func (s *service) run(deployments []Deployment) {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

	for _, deployment := range deployments {
		key := deployment.LeaseID().String()
		s.managers[key] = newDeploymentManager(s, deployment.LeaseID(), deployment.ManifestGroup())
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

				if _, err := s.inventory.lookup(ev.LeaseID.OrderID(), mgroup); err != nil {
					s.log.Error("error looking up manifest", "err", err, "lease", ev.LeaseID, "group-name", mgroup.Name)
					break
				}

				key := ev.LeaseID.String()

				if manager := s.managers[key]; manager != nil {
					if err := manager.update(mgroup); err != nil {
						s.log.Error("updating deployment", "err", err, "lease", ev.LeaseID, "group-name", mgroup.Name)
					}
					break
				}

				manager := newDeploymentManager(s, ev.LeaseID, mgroup)
				s.managers[key] = manager

			case *event.TxCloseDeployment:

				// teardown/undeploy managed deployments

				for _, manager := range s.managers {
					if bytes.Equal(ev.Deployment, manager.lease.OrderID().DeploymentID()) {
						s.teardownLease(manager.lease)
					}
				}

			case *event.TxCloseFulfillment:

				s.teardownLease(ev.FulfillmentID.LeaseID())

			case *event.TxCloseLease:

				s.teardownLease(ev.LeaseID)

			}

		case dm := <-s.managerch:

			s.log.Debug("manager done", "lease", dm.lease)

			if _, err := s.inventory.unreserve(dm.lease.OrderID(), dm.mgroup); err != nil {
				s.log.Error("unreserving inventory", "err", err,
					"lease", dm.lease, "group-name", dm.mgroup.Name)
			}

			// todo: unreserve resources

			delete(s.managers, dm.lease.String())
		}
	}

	s.log.Debug("draining deployment managers...")

	for _, manager := range s.managers {
		if manager != nil {
			manager := <-s.managerch
			s.log.Debug("manager done", "lease", manager.lease)
		}
	}

	<-s.inventory.done()

}

func (s *service) teardownLease(lid types.LeaseID) {
	key := lid.String()
	manager := s.managers[key]
	if manager == nil {
		return
	}

	if err := manager.teardown(); err != nil {
		s.log.Error("tearing down lease deployment", "err", err, "lease", lid)
	}
}
