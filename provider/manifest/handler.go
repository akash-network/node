package manifest

import (
	"context"
	"errors"
	"sync"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/ovrclk/akash/util/runner"
)

var ErrNotRunning = errors.New("not running")

type Handler interface {
	HandleManifest(*types.ManifestRequest) error
}

type Service interface {
	Handler
	Done() <-chan struct{}
}

func NewHandler(ctx context.Context, session session.Session, bus event.Bus) (Service, error) {
	sub, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}

	h := &handler{
		session:      session.ForModule("provider-manifest"),
		bus:          bus,
		sub:          sub,
		mreqch:       make(chan manifestRequest),
		mstates:      make(map[string]*manifestState),
		deploymentch: make(chan runner.Result),
		wg:           sync.WaitGroup{},
		lc:           lifecycle.New(),
	}

	go h.lc.WatchContext(ctx)
	go h.run()

	return h, nil
}

type handler struct {
	session session.Session
	bus     event.Bus
	sub     event.Subscriber

	mreqch  chan manifestRequest
	mstates map[string]*manifestState

	deploymentch chan runner.Result

	wg sync.WaitGroup
	lc lifecycle.Lifecycle
}

type manifestState struct {
	request *types.ManifestRequest
	leaseID *types.LeaseID

	deployment        *types.Deployment
	deploymentPending bool
}

func (mstate *manifestState) complete() bool {
	return mstate.request != nil &&
		mstate.leaseID != nil &&
		mstate.deployment != nil
}

type manifestRequest struct {
	value *types.ManifestRequest
	ch    chan<- error
}

func (h *handler) HandleManifest(mreq *types.ManifestRequest) error {
	ch := make(chan error, 1)
	req := manifestRequest{mreq, ch}
	select {
	case h.mreqch <- req:
		return <-ch
	case <-h.lc.ShuttingDown():
		return ErrNotRunning
	}
}

func (h *handler) Done() <-chan struct{} {
	return h.lc.Done()
}

func (h *handler) run() {
	defer h.lc.ShutdownCompleted()
	defer h.sub.Close()

	ctx, cancel := context.WithCancel(context.Background())

loop:
	for {
		select {

		case err := <-h.lc.ShutdownRequest():
			h.lc.ShutdownInitiated(err)
			break loop

		case ev := <-h.sub.Events():
			switch ev := ev.(type) {
			case event.LeaseWon:
				did := ev.LeaseID.Deployment
				mstate := h.getManifestState(did)

				h.session.Log().Info("lease won", "deployment", did.EncodeString())

				// TODO: validate single lease
				mstate.leaseID = &ev.LeaseID

				h.checkManifestState(ctx, mstate, did)

			case *event.TxCloseDeployment:
				// TODO

			case *event.TxCloseFulfillment:
				// TODO
			}

		case req := <-h.mreqch:
			// new manifest received

			did := req.value.Deployment
			mstate := h.getManifestState(did)

			h.session.Log().Info("manifest received", "deployment", did.EncodeString())

			// TODO: validate single manifest
			mstate.request = req.value

			h.checkManifestState(ctx, mstate, did)

			// TODO: defer response until validation
			req.ch <- nil

		case req := <-h.deploymentch:

			if err := req.Error(); err != nil {
				h.session.Log().Error("fetching deployment", "err", err)
				break
			}

			deployment := req.Value().(*types.Deployment)
			did := deployment.Address
			key := did.EncodeString()

			mstate := h.mstates[key]

			if mstate == nil {
				h.session.Log().Error("rogue deployment received", "deployment", key)
				break
			}

			mstate.deployment = deployment
			mstate.deploymentPending = false

			h.session.Log().Info("deployment received", "deployment", key)

			h.checkManifestState(ctx, mstate, did)

		}
	}
	cancel()
	h.wg.Wait()
}

func (h *handler) getManifestState(did base.Bytes) *manifestState {
	key := did.EncodeString()
	mstate := h.mstates[key]

	if mstate == nil {
		mstate = &manifestState{}
		h.mstates[key] = mstate
	}

	return mstate
}

func (h *handler) checkManifestState(ctx context.Context, mstate *manifestState, did base.Bytes) {
	if mstate.complete() {

		// TODO: validate manifest

		// publish complete manifest
		h.bus.Publish(event.ManifestReceived{
			LeaseID:    *mstate.leaseID,
			Manifest:   mstate.request.Manifest,
			Deployment: mstate.deployment,
		})
		h.session.Log().Debug("manifest complete", "deployment", did.EncodeString())
		return
	}

	if mstate.deployment == nil && !mstate.deploymentPending {
		mstate.deploymentPending = true
		h.fetchDeployment(ctx, did)
		return
	}
}

func (h *handler) fetchDeployment(ctx context.Context, key base.Bytes) {
	h.session.Log().Debug("fetching deployment", "deployment", key.EncodeString())
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		res, err := h.session.Query().Deployment(ctx, key)
		select {
		case h.deploymentch <- runner.NewResult(res, err):
		case <-h.lc.ShuttingDown():
		}
	}()
}
