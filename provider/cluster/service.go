package cluster

import (
	"context"

	"github.com/boz/go-lifecycle"
	sdktypes "github.com/cosmos/cosmos-sdk/types"

	v1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/tendermint/tendermint/libs/log"

	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	atypes "github.com/ovrclk/akash/types"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// ErrNotRunning is the error when service is not running
var ErrNotRunning = errors.New("not running")

var (
	deploymentManagerGauge = promauto.NewGauge(prometheus.GaugeOpts{
		// fixme provider_deployment_manager
		Name:        "provider_deploymetn_manager",
		Help:        "",
		ConstLabels: nil,
	})
)

// Cluster is the interface that wraps Reserve and Unreserve methods
type Cluster interface {
	Reserve(mtypes.OrderID, atypes.ResourceGroup) (ctypes.Reservation, error)
	Unreserve(mtypes.OrderID) error
}

// StatusClient is the interface which includes status of service
type StatusClient interface {
	Status(context.Context) (*ctypes.Status, error)
	FindActiveLease(ctx context.Context, owner sdktypes.Address, dseq uint64, gseq uint32) (bool, mtypes.LeaseID, v1.ManifestGroup, error)
}

// Service manage compute cluster for the provider.  Will eventually integrate with kubernetes, etc...
type Service interface {
	StatusClient
	Cluster
	Close() error
	Ready() <-chan struct{}
	Done() <-chan struct{}
	HostnameService() ctypes.HostnameServiceClient
	TransferHostname(ctx context.Context, leaseID mtypes.LeaseID, hostname string, serviceName string, externalPort uint32) error
}

// NewService returns new Service instance
func NewService(ctx context.Context, session session.Session, bus pubsub.Bus, client Client, cfg Config) (Service, error) {
	log := session.Log().With("module", "provider-cluster", "cmp", "service")

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

	inventory, err := newInventoryService(cfg, log, lc.ShuttingDown(), sub, client, deployments)
	if err != nil {
		sub.Close()
		return nil, err
	}

	allHostnames, err := client.AllHostnames(ctx)
	if err != nil {
		sub.Close()
		return nil, err
	}

	// Note: one side effect of this code is to add reservations for auto generated hostnames
	// This is not normally done, but also doesn't cause any problems
	activeHostnames := make(map[string]mtypes.LeaseID, len(allHostnames))
	for _, v := range allHostnames {
		activeHostnames[v.Hostname] = v.ID
		log.Debug("found existing hostname", "hostname", v.Hostname, "id", v.ID)
	}
	hostnames, err := newHostnameService(ctx, cfg, activeHostnames)
	if err != nil {
		return nil, err
	}

	s := &service{
		session:                        session,
		client:                         client,
		hostnames:                      hostnames,
		bus:                            bus,
		sub:                            sub,
		inventory:                      inventory,
		statusch:                       make(chan chan<- *ctypes.Status),
		managers:                       make(map[mtypes.LeaseID]*deploymentManager),
		managerch:                      make(chan *deploymentManager),
		checkDeploymentExistsRequestCh: make(chan checkDeploymentExistsRequest),

		log:    log,
		lc:     lc,
		config: cfg,
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
	hostnames *hostnameService

	checkDeploymentExistsRequestCh chan checkDeploymentExistsRequest
	statusch                       chan chan<- *ctypes.Status
	managers                       map[mtypes.LeaseID]*deploymentManager

	managerch chan *deploymentManager

	log log.Logger
	lc  lifecycle.Lifecycle

	config Config
}

type checkDeploymentExistsRequest struct {
	owner sdktypes.Address
	dseq  uint64
	gseq  uint32

	responseCh chan<- mtypes.LeaseID
}

var errNoManifestGroup = errors.New("no manifest group could be found")

func (s *service) FindActiveLease(ctx context.Context, owner sdktypes.Address, dseq uint64, gseq uint32) (bool, mtypes.LeaseID, v1.ManifestGroup, error) {
	response := make(chan mtypes.LeaseID, 1)
	req := checkDeploymentExistsRequest{
		responseCh: response,
		dseq:       dseq,
		gseq:       gseq,
		owner:      owner,
	}
	select {
	case s.checkDeploymentExistsRequestCh <- req:
	case <-ctx.Done():
		return false, mtypes.LeaseID{}, v1.ManifestGroup{}, ctx.Err()
	}

	var leaseID mtypes.LeaseID
	var ok bool
	select {
	case leaseID, ok = <-response:
		if !ok {
			return false, mtypes.LeaseID{}, v1.ManifestGroup{}, nil
		}

	case <-ctx.Done():
		return false, mtypes.LeaseID{}, v1.ManifestGroup{}, ctx.Err()
	}

	found, mgroup, err := s.client.GetManifestGroup(ctx, leaseID)
	if err != nil {
		return false, mtypes.LeaseID{}, v1.ManifestGroup{}, err
	}

	if !found {
		return false, mtypes.LeaseID{}, v1.ManifestGroup{}, errNoManifestGroup
	}

	return true, leaseID, mgroup, nil
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

func (s *service) Reserve(order mtypes.OrderID, resources atypes.ResourceGroup) (ctypes.Reservation, error) {
	return s.inventory.reserve(order, resources)
}

func (s *service) Unreserve(order mtypes.OrderID) error {
	return s.inventory.unreserve(order)
}

func (s *service) HostnameService() ctypes.HostnameServiceClient {
	return s.hostnames
}

func (s *service) TransferHostname(ctx context.Context, leaseID mtypes.LeaseID, hostname string, serviceName string, externalPort uint32) error {
	return s.client.DeclareHostname(ctx, leaseID, hostname, serviceName, externalPort)
}

func (s *service) Status(ctx context.Context) (*ctypes.Status, error) {
	istatus, err := s.inventory.status(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan *ctypes.Status, 1)

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

func (s *service) updateDeploymentManagerGauge() {
	deploymentManagerGauge.Set(float64(len(s.managers)))
}

func (s *service) run(deployments []ctypes.Deployment) {
	defer s.lc.ShutdownCompleted()
	defer s.sub.Close()

	s.updateDeploymentManagerGauge()
	for _, deployment := range deployments {
		key := deployment.LeaseID()
		mgroup := deployment.ManifestGroup()
		s.managers[key] = newDeploymentManager(s, deployment.LeaseID(), &mgroup)
		s.updateDeploymentManagerGauge()
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
					s.log.Error("indeterminate manifest group", "lease", ev.LeaseID, "group-name", ev.Group.GroupSpec.Name)
					break
				}

				if _, err := s.inventory.lookup(ev.LeaseID.OrderID(), mgroup); err != nil {
					s.log.Error("error looking up manifest", "err", err, "lease", ev.LeaseID, "group-name", mgroup.Name)
					break
				}

				key := ev.LeaseID
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
			ch <- &ctypes.Status{
				Leases: uint32(len(s.managers)),
			}
		case dm := <-s.managerch:
			s.log.Info("manager done", "lease", dm.lease)

			// unreserve resources
			if err := s.inventory.unreserve(dm.lease.OrderID()); err != nil {
				s.log.Error("unreserving inventory", "err", err,
					"lease", dm.lease)
			}

			delete(s.managers, dm.lease)
		case req := <-s.checkDeploymentExistsRequestCh:
			s.doCheckDeploymentExists(req)
		}
		s.updateDeploymentManagerGauge()
	}
	s.log.Debug("draining deployment managers...", "qty", len(s.managers))
	for _, manager := range s.managers {
		if manager != nil {
			manager := <-s.managerch
			s.log.Debug("manager done", "lease", manager.lease)
		}
	}

	<-s.inventory.done()

}

func (s *service) doCheckDeploymentExists(req checkDeploymentExistsRequest) {
	for leaseID := range s.managers {
		// Check for a match
		if leaseID.GSeq == req.gseq && leaseID.DSeq == req.dseq && leaseID.Owner == req.owner.String() {
			req.responseCh <- leaseID
			return
		}
	}

	close(req.responseCh)
}

func (s *service) teardownLease(lid mtypes.LeaseID) {
	if manager := s.managers[lid]; manager != nil {
		if err := manager.teardown(); err != nil {
			s.log.Error("tearing down lease deployment", "err", err, "lease", lid)
		}
		return
	}

	// unreserve resources if no manager present yet.
	if lid.Provider == s.session.Provider().Owner {
		s.log.Info("unreserving unmanaged order", "lease", lid)
		err := s.inventory.unreserve(lid.OrderID())
		if err != nil && !errors.Is(errReservationNotFound, err) {
			s.log.Error("unreserve failed", "lease", lid, "err", err)
		}
	}
}

func findDeployments(ctx context.Context, log log.Logger, client Client, session session.Session) ([]ctypes.Deployment, error) {
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

	leases := make(map[string]bool)
	for _, lease := range leaseList {
		leases[mquery.LeasePath(lease.Lease.LeaseID)] = true
	}

	log.Info("found leases", "num-active", len(leases))

	active := make([]ctypes.Deployment, 0, len(deployments))

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
