package cluster

import (
	"context"
	"errors"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/caarlos0/env"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	atypes "github.com/ovrclk/akash/types"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"
)

// ErrNotRunning is the error when service is not running
var ErrNotRunning = errors.New("not running")

// Cluster is the interface that wraps Reserve and Unreserve methods
type Cluster interface {
	Reserve(mtypes.OrderID, atypes.ResourceGroup) (Reservation, error)
	Unreserve(mtypes.OrderID, atypes.ResourceGroup) error
}

// StatusClient is the interface which includes status of service
type StatusClient interface {
	Status(context.Context) (*Status, error)
}

// Service manage compute cluster for the provider.  Will eventually integrate with kubernetes, etc...
type Service interface {
	StatusClient
	Cluster
	Close() error
	Ready() <-chan struct{}
	Done() <-chan struct{}
}

// NewService returns new Service instance
func NewService(ctx context.Context, session session.Session, bus pubsub.Bus, client Client) (Service, error) {
	log := session.Log().With("module", "provider-cluster", "cmp", "service")

	config := config{}
	if err := env.Parse(&config); err != nil {
		log.Error("parsing config", "err", err)
		return nil, err
	}

	lc := lifecycle.New()

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	deployments, err := findDeployments(ctx, log, client, session)
	if err != nil {
		sub.Close()
		return nil, err
	}

	inventory, err := newInventoryService(config, log, lc.ShuttingDown(), sub, client, deployments)
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
		statusch:  make(chan chan<- *Status),
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
	bus     pubsub.Bus
	sub     pubsub.Subscriber

	inventory *inventoryService

	statusch  chan chan<- *Status
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

func (s *service) Ready() <-chan struct{} {
	return s.inventory.ready()
}

func (s *service) Reserve(order mtypes.OrderID, resources atypes.ResourceGroup) (Reservation, error) {
	return s.inventory.reserve(order, resources)
}

func (s *service) Unreserve(order mtypes.OrderID, resources atypes.ResourceGroup) error {
	_, err := s.inventory.unreserve(order, resources)
	return err
}

func (s *service) Status(ctx context.Context) (*Status, error) {

	istatus, err := s.inventory.status(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan *Status, 1)

	select {
	case <-s.lc.Done():
		return nil, ErrNotRunning
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.statusch <- ch:
	}

	select {
	case <-s.lc.Done():
		return nil, ErrNotRunning
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		result.Inventory = istatus
		return result, nil
	}

}

func (s *service) run(deployments []Deployment) {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

	for _, deployment := range deployments {
		key := mquery.LeasePath(deployment.LeaseID())
		mgroup := deployment.ManifestGroup()
		s.managers[key] = newDeploymentManager(s, deployment.LeaseID(), &mgroup)
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

				key := mquery.LeasePath(ev.LeaseID)

				if manager := s.managers[key]; manager != nil {
					if err := manager.update(mgroup); err != nil {
						s.log.Error("updating deployment", "err", err, "lease", ev.LeaseID, "group-name", mgroup.Name)
					}
					break
				}

				manager := newDeploymentManager(s, ev.LeaseID, mgroup)
				s.managers[key] = manager

			case mtypes.EventLeaseClosed:

				s.teardownLease(ev.ID)

			}

		case ch := <-s.statusch:

			ch <- &Status{
				Leases: uint32(len(s.managers)),
			}

		case dm := <-s.managerch:

			s.log.Debug("manager done", "lease", dm.lease)

			if _, err := s.inventory.unreserve(dm.lease.OrderID(), dm.mgroup); err != nil {
				s.log.Error("unreserving inventory", "err", err,
					"lease", dm.lease, "group-name", dm.mgroup.Name)
			}

			// todo: unreserve resources

			delete(s.managers, mquery.LeasePath(dm.lease))
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

func (s *service) teardownLease(lid mtypes.LeaseID) {
	key := mquery.LeasePath(lid)
	manager := s.managers[key]
	if manager == nil {
		return
	}

	if err := manager.teardown(); err != nil {
		s.log.Error("tearing down lease deployment", "err", err, "lease", lid)
	}
}

func findDeployments(ctx context.Context, log log.Logger, client Client, session session.Session) ([]Deployment, error) {
	deployments, err := client.Deployments(ctx)
	if err != nil {
		log.Error("fetching deployments", "err", err)
		return nil, err
	}

	leaseList, err := session.Client().Query().ActiveLeasesForProvider(session.Provider().Address())
	if err != nil {
		log.Error("fetching deployments", "err", err)
		return nil, err
	}

	var leases map[string]bool

	for _, lease := range leaseList {
		leases[mquery.LeasePath(lease.LeaseID)] = true
	}

	log.Info("found leases", "num-active", len(leases))

	active := make([]Deployment, 0, len(deployments))

	for _, deployment := range deployments {
		if _, ok := leases[mquery.LeasePath(deployment.LeaseID())]; !ok {
			continue
		}
		active = append(active, deployment)
		log.Debug("deployment", "lease", deployment.LeaseID(), "mgroup", deployment.ManifestGroup().Name)
	}

	log.Info("found deployments", "num-active", len(active), "num-skipped", len(deployments)-len(active))

	return active, nil
}
