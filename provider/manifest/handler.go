package manifest

import (
	"bytes"
	"context"
	"errors"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/caarlos0/env"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/util/runner"
	dquery "github.com/ovrclk/akash/x/deployment/query"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

var ErrNotRunning = errors.New("not running")

type StatusClient interface {
	Status(context.Context) (*Status, error)
}

type Handler interface {
	HandleManifest(context.Context, *manifest.Request) error
}

type Service interface {
	StatusClient
	Handler
	Done() <-chan struct{}
}

// Manage incoming leases and manifests and pair the two together to construct and emit a ManifestReceived event.
func NewHandler(ctx context.Context, session session.Session, bus pubsub.Bus) (Service, error) {

	session = session.ForModule("provider-manifest")

	config := config{}
	if err := env.Parse(&config); err != nil {
		session.Log().Error("parsing config", "err", err)
		return nil, err
	}

	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	leases, err := fetchExistingLeases(ctx, session)
	if err != nil {
		session.Log().Error("fetching existing leases", "err", err)
		sub.Close()
		return nil, err
	}
	session.Log().Info("found existing leases", "count", len(leases))

	h := &handler{
		session:   session,
		bus:       bus,
		sub:       sub,
		statusch:  make(chan chan<- *Status),
		mreqch:    make(chan manifestRequest),
		managers:  make(map[string]*manager),
		managerch: make(chan *manager),
		lc:        lifecycle.New(),
	}

	go h.lc.WatchContext(ctx)
	go h.run(leases)

	return h, nil
}

type handler struct {
	config  config
	session session.Session
	bus     pubsub.Bus
	sub     pubsub.Subscriber

	statusch chan chan<- *Status
	mreqch   chan manifestRequest

	managers  map[string]*manager
	managerch chan *manager

	deploymentch chan runner.Result

	lc lifecycle.Lifecycle
}

type manifestRequest struct {
	value *manifest.Request
	ch    chan<- error
	ctx   context.Context
}

// Send incoming manifest request.
func (h *handler) HandleManifest(ctx context.Context, mreq *manifest.Request) error {
	ch := make(chan error, 1)
	req := manifestRequest{value: mreq, ch: ch, ctx: ctx}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case h.mreqch <- req:
		return <-ch
	case <-h.lc.ShuttingDown():
		return ErrNotRunning
	}
}

func (h *handler) Done() <-chan struct{} {
	return h.lc.Done()
}

func (h *handler) Status(ctx context.Context) (*Status, error) {
	ch := make(chan *Status, 1)

	select {
	case <-h.lc.Done():
		return nil, ErrNotRunning
	case <-ctx.Done():
		return nil, ctx.Err()
	case h.statusch <- ch:
	}

	select {
	case <-h.lc.Done():
		return nil, ErrNotRunning
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		return result, nil
	}
}

func (h *handler) run(leases []event.LeaseWon) {
	defer h.lc.ShutdownCompleted()
	defer h.sub.Close()

	h.managePreExistingLease(leases)

loop:
	for {
		select {

		case err := <-h.lc.ShutdownRequest():
			h.lc.ShutdownInitiated(err)
			break loop

		case ev := <-h.sub.Events():
			switch ev := ev.(type) {

			case event.LeaseWon:
				h.session.Log().Info("lease won", "lease", ev.LeaseID)

				h.handleLease(ev)

			case dtypes.EventDeploymentUpdate:

				h.session.Log().Info("update received", "deployment", ev.ID, "version", ev.Version)

				key := dquery.DeploymentPath(ev.ID)
				if manager := h.managers[key]; manager != nil {
					h.session.Log().Info("deployment updated", "deployment", ev.ID, "version", ev.Version)
					manager.handleUpdate(ev.Version)
				}

			case dtypes.EventDeploymentClose:

				key := dquery.DeploymentPath(ev.ID)
				if manager := h.managers[key]; manager != nil {
					h.session.Log().Info("deployment closed", "deployment", ev.ID)
					manager.stop()
				}

			case mtypes.EventLeaseClosed:

				if !bytes.Equal(ev.ID.Provider, h.session.Provider()) {
					continue
				}

				key := dquery.DeploymentPath(ev.ID.DeploymentID())
				if manager := h.managers[key]; manager != nil {
					h.session.Log().Info("lease closed", "lease", ev.ID)
					manager.removeLease(ev.ID)
				}

			}

		// case req := <-h.mreqch:
		// TODO

		// if err := validation.ValidateManifest(req.value.Manifest); err != nil {
		// 	h.session.Log().Error("manifest validation failed",
		// 		"err", err, "deployment", req.value.Deployment)
		// 	req.ch <- err
		// 	break
		// }

		// manager, err := h.ensureManger(req.value.Deployment)
		// if err != nil {
		// 	h.session.Log().Error("error fetching manager for manifest",
		// 		"err", err, "deployment", req.value.Deployment)
		// 	req.ch <- err
		// 	break
		// }

		// manager.handleManifest(req)

		case ch := <-h.statusch:

			ch <- &Status{
				Deployments: uint32(len(h.managers)),
			}

		case manager := <-h.managerch:
			h.session.Log().Info("manager done", "deployment", manager.daddr)

			delete(h.managers, dquery.DeploymentPath(manager.daddr))
		}
	}

	for len(h.managers) > 0 {
		manager := <-h.managerch
		delete(h.managers, dquery.DeploymentPath(manager.daddr))
	}

}

func (h *handler) managePreExistingLease(leases []event.LeaseWon) {
	for _, lease := range leases {
		h.handleLease(lease)
	}
}

func (h *handler) handleLease(ev event.LeaseWon) {
	manager, err := h.ensureManger(ev.LeaseID.DeploymentID())
	if err != nil {
		h.session.Log().Error("error creating manager",
			"err", err, "lease", ev.LeaseID)
		return
	}

	manager.handleLease(ev)
}

func (h *handler) ensureManger(did dtypes.DeploymentID) (manager *manager, err error) {
	manager = h.managers[dquery.DeploymentPath(did)]
	if manager == nil {
		manager, err = newManager(h, did)
		if err != nil {
			return nil, err
		}
		h.managers[dquery.DeploymentPath(did)] = manager
	}
	return manager, nil
}

func fetchExistingLeases(ctx context.Context, session session.Session) ([]event.LeaseWon, error) {
	leases, err := session.Client().Query().ActiveLeasesForProvider(session.Provider())
	if err != nil {
		return nil, err
	}

	var items []event.LeaseWon

	for _, lease := range leases {

		dgroup, err := session.Client().Query().Group(lease.GroupID())
		if err != nil {
			session.Log().Error("can't fetch deployment group", "err", err, "lease", lease)
			continue
		}

		items = append(items, event.LeaseWon{
			LeaseID: lease.LeaseID,
			Price:   lease.Price,
			Group:   &dgroup,
		})
	}

	session.Log().Debug("fetching leases", "lease-count", len(items))

	return items, nil
}
