package manifest

import (
	"bytes"
	"context"
	"errors"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/caarlos0/env"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/ovrclk/akash/util/runner"
	"github.com/ovrclk/akash/validation"
)

var ErrNotRunning = errors.New("not running")

type Handler interface {
	HandleManifest(context.Context, *types.ManifestRequest) error
}

type Service interface {
	Handler
	Done() <-chan struct{}
}

// Manage incoming leases and manifests and pair the two together to construct and emit a ManifestReceived event.
func NewHandler(ctx context.Context, session session.Session, bus event.Bus) (Service, error) {

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
	bus     event.Bus
	sub     event.Subscriber

	mreqch chan manifestRequest

	managers  map[string]*manager
	managerch chan *manager

	deploymentch chan runner.Result

	lc lifecycle.Lifecycle
}

type manifestRequest struct {
	value *types.ManifestRequest
	ch    chan<- error
	ctx   context.Context
}

// Send incoming manifest request.
func (h *handler) HandleManifest(ctx context.Context, mreq *types.ManifestRequest) error {
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

			case *event.TxUpdateDeployment:

				h.session.Log().Info("update received", "deployment", ev.Deployment, "version", ev.Version)

				key := ev.Deployment.String()
				if manager := h.managers[key]; manager != nil {
					h.session.Log().Info("deployment updated", "deployment", ev.Deployment, "version", ev.Version)

					manager.handleUpdate(ev.Version)
				}

			case *event.TxCloseDeployment:

				key := ev.Deployment.String()
				if manager := h.managers[key]; manager != nil {
					h.session.Log().Info("deployment closed", "deployment", ev.Deployment)

					manager.stop()
				}

			case *event.TxCloseFulfillment:

				if bytes.Equal(ev.Provider, h.session.Provider().Address) {
					key := ev.Deployment.String()
					if manager := h.managers[key]; manager != nil {
						h.session.Log().Info("fulfillment closed", "fulfillment", ev.FulfillmentID)
						manager.removeLease(ev.FulfillmentID.LeaseID())
					}
				}

			case *event.TxCloseLease:

				if bytes.Equal(ev.Provider, h.session.Provider().Address) {
					key := ev.Deployment.String()
					if manager := h.managers[key]; manager != nil {
						h.session.Log().Info("lease closed", "lease", ev.LeaseID)
						manager.removeLease(ev.LeaseID)
					}
				}

			}

		case req := <-h.mreqch:

			if err := validation.ValidateManifest(req.value.Manifest); err != nil {
				h.session.Log().Error("manifest validation failed",
					"err", err, "deployment", req.value.Deployment)
				req.ch <- err
				break
			}

			manager, err := h.ensureManger(req.value.Deployment)
			if err != nil {
				h.session.Log().Error("error fetching manager for manifest",
					"err", err, "deployment", req.value.Deployment)
				req.ch <- err
				break
			}

			manager.handleManifest(req)

		case manager := <-h.managerch:
			h.session.Log().Info("manager done", "deployment", manager.daddr)

			delete(h.managers, manager.daddr.String())
		}
	}

	for len(h.managers) > 0 {
		manager := <-h.managerch
		delete(h.managers, manager.daddr.String())
	}

}

func (h *handler) managePreExistingLease(leases []event.LeaseWon) {
	for _, lease := range leases {
		h.handleLease(lease)
	}
}

func (h *handler) handleLease(ev event.LeaseWon) {
	manager, err := h.ensureManger(ev.LeaseID.Deployment)
	if err != nil {
		h.session.Log().Error("error creating manager",
			"err", err, "lease", ev.LeaseID)
		return
	}

	manager.handleLease(ev)
}

func (h *handler) ensureManger(did base.Bytes) (manager *manager, err error) {
	key := did.String()
	manager = h.managers[key]
	if manager == nil {
		manager, err = newManager(h, did)
		if err != nil {
			return nil, err
		}
		h.managers[key] = manager
	}
	return manager, nil
}

func fetchExistingLeases(ctx context.Context, session session.Session) ([]event.LeaseWon, error) {
	leases, err := session.Query().ProviderLeases(ctx, session.Provider().Address)
	if err != nil {
		return nil, err
	}

	var items []event.LeaseWon

	for _, lease := range leases.Items {
		if lease.State != types.Lease_ACTIVE {
			continue
		}

		dgroup, err := session.Query().DeploymentGroup(ctx, lease.LeaseID.GroupID())
		if err != nil {
			session.Log().Error("can't fetch deployment group", "err", err, "lease", lease)
			continue
		}

		items = append(items, event.LeaseWon{
			LeaseID: lease.LeaseID,
			Price:   lease.Price,
			Group:   dgroup,
		})
	}

	session.Log().Debug("fetching leases", "lease-count", len(items))

	return items, nil
}
