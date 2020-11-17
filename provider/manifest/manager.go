package manifest

import (
	"bytes"
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/libs/log"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/util/runner"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

var (
	// ErrShutdownTimerExpired for a terminating deployment
	ErrShutdownTimerExpired = errors.New("shutdown timer expired")
	// ErrManifestVersion indicates that the given manifest's version does not
	// match the blockchain Version value.
	ErrManifestVersion = errors.New("manifest version validation failed")
)

func newManager(h *service, daddr dtypes.DeploymentID) (*manager, error) {
	session := h.session.ForModule("manifest-manager")

	sub, err := h.sub.Clone()
	if err != nil {
		return nil, err
	}

	m := &manager{
		config:     h.config,
		daddr:      daddr,
		session:    session,
		bus:        h.bus,
		sub:        sub,
		leasech:    make(chan event.LeaseWon),
		rmleasech:  make(chan mtypes.LeaseID),
		manifestch: make(chan manifestRequest),
		updatech:   make(chan []byte),
		log:        session.Log().With("deployment", daddr),
		lc:         lifecycle.New(),
	}

	go m.lc.WatchChannel(h.lc.ShuttingDown())
	go m.run(h.managerch)

	return m, nil
}

// 'manager' facilitates operations around a configured Deployment.
type manager struct {
	config  config
	daddr   dtypes.DeploymentID
	session session.Session
	bus     pubsub.Bus
	sub     pubsub.Subscriber

	leasech    chan event.LeaseWon
	rmleasech  chan mtypes.LeaseID
	manifestch chan manifestRequest
	updatech   chan []byte

	data      *dtypes.DeploymentResponse
	requests  []manifestRequest
	leases    []event.LeaseWon
	manifests []*manifest.Manifest
	versions  [][]byte

	stoptimer *time.Timer

	log log.Logger
	lc  lifecycle.Lifecycle
}

func (m *manager) stop() {
	m.lc.ShutdownAsync(nil)
}

func (m *manager) handleLease(ev event.LeaseWon) {
	select {
	case m.leasech <- ev:
	case <-m.lc.ShuttingDown():
		m.log.Error("not running: handle manifest", "lease", ev.LeaseID)
	}
}

func (m *manager) removeLease(id mtypes.LeaseID) {
	select {
	case m.rmleasech <- id:
	case <-m.lc.ShuttingDown():
		m.log.Error("not running: remove lease", "lease", id)
	}
}

func (m *manager) handleManifest(req manifestRequest) {
	select {
	case m.manifestch <- req:
	case <-m.lc.ShuttingDown():
		m.log.Error("not running: handle manifest")
		req.ch <- ErrNotRunning
	}
}

func (m *manager) handleUpdate(version []byte) {
	select {
	case m.updatech <- version:
	case <-m.lc.ShuttingDown():
		m.log.Error("not running: version update", "version", version)
	}
}

func (m *manager) run(donech chan<- *manager) {
	defer m.lc.ShutdownCompleted()
	defer func() { donech <- m }()

	var runch <-chan runner.Result

	ctx, cancel := context.WithCancel(context.Background())

loop:
	for {

		var stopch <-chan time.Time
		if m.stoptimer != nil {
			stopch = m.stoptimer.C
		}

		select {

		case err := <-m.lc.ShutdownRequest():
			m.lc.ShutdownInitiated(err)
			break loop

		case <-stopch:
			m.log.Error(ErrShutdownTimerExpired.Error())
			m.lc.ShutdownInitiated(ErrShutdownTimerExpired)
			break loop

		case ev := <-m.leasech:
			m.log.Info("new lease", "lease", ev.LeaseID)

			m.leases = append(m.leases, ev)
			m.emitReceivedEvents()
			m.maybeScheduleStop()
			runch = m.maybeFetchData(ctx, runch)

		case id := <-m.rmleasech:
			m.log.Info("lease removed", "lease", id)

			for idx, lease := range m.leases {
				if id.Equals(lease.LeaseID) {
					m.leases = append(m.leases[:idx], m.leases[idx+1:]...)
				}
			}

			m.maybeScheduleStop()

		case req := <-m.manifestch:
			m.log.Info("manifest received")

			// TODO: fail fast if invalid request to prevent DoS

			m.requests = append(m.requests, req)
			m.validateRequests()
			m.emitReceivedEvents()
			m.maybeScheduleStop()
			runch = m.maybeFetchData(ctx, runch)

		case version := <-m.updatech:
			m.log.Info("received version", "version", version)
			m.versions = append(m.versions, version)
			if m.data != nil {
				m.data.Deployment.Version = version
			}

		case result := <-runch:
			runch = nil

			if err := result.Error(); err != nil {
				m.log.Error("error fetching data", "err", err)
				break
			}

			m.data = result.Value().(*dtypes.DeploymentResponse)

			m.log.Info("data received", "version", m.data.Deployment.Version)

			m.validateRequests()
			m.emitReceivedEvents()
			m.maybeScheduleStop()

		}
	}

	cancel()

	for _, req := range m.requests {
		req.ch <- ErrNotRunning
	}

	if m.stoptimer != nil {
		if m.stoptimer.Stop() {
			<-m.stoptimer.C
		}
	}

	if runch != nil {
		<-runch
	}

}

func (m *manager) maybeFetchData(ctx context.Context, runch <-chan runner.Result) <-chan runner.Result {
	if m.data == nil && runch == nil {
		return m.fetchData(ctx)
	}
	return runch
}

func (m *manager) fetchData(ctx context.Context) <-chan runner.Result {
	return runner.Do(func() runner.Result {
		// TODO: retry
		return runner.NewResult(m.doFetchData(ctx))
	})
}

func (m *manager) doFetchData(_ context.Context) (*dtypes.DeploymentResponse, error) {
	res, err := m.session.Client().Query().Deployment(context.Background(), &dtypes.QueryDeploymentRequest{ID: m.daddr})
	if err != nil {
		return nil, err
	}
	return &res.Deployment, nil
}

func (m *manager) maybeScheduleStop() bool { // nolint:golint,unparam
	if len(m.leases) > 0 || len(m.manifests) > 0 {
		if m.stoptimer != nil {
			m.log.Info("stopping stop timer")
			if m.stoptimer.Stop() {
				<-m.stoptimer.C
			}
			m.stoptimer = nil
		}
		return false
	}
	if m.stoptimer != nil {
		m.log.Info("starting stop timer", "duration", m.config.ManifestLingerDuration)
		m.stoptimer = time.NewTimer(m.config.ManifestLingerDuration)
	}
	return true
}

func (m *manager) emitReceivedEvents() {
	if m.data == nil || len(m.leases) == 0 || len(m.manifests) == 0 {
		m.log.Debug("emit received events skipped", "data", m.data, "leases", len(m.leases), "manifests", len(m.manifests))
		return
	}

	manifest := m.manifests[len(m.manifests)-1]

	m.log.Debug("publishing manifest received", "num-leases", len(m.leases))

	for _, lease := range m.leases {
		if err := m.bus.Publish(event.ManifestReceived{
			LeaseID:    lease.LeaseID,
			Group:      lease.Group,
			Manifest:   manifest,
			Deployment: m.data,
		}); err != nil {
			m.log.Error("publishing event", "err", err, "lease", lease.LeaseID)
		}
	}
}

func (m *manager) validateRequests() {
	if m.data == nil || len(m.requests) == 0 {
		return
	}

	manifests := make([]*manifest.Manifest, 0)
	for _, req := range m.requests {
		if err := m.validateRequest(req); err != nil {
			m.log.Error("invalid manifest", "err", err)
			req.ch <- err
			continue
		}
		manifests = append(manifests, &req.value.Manifest)
		req.ch <- nil
	}
	m.requests = nil

	m.log.Debug("requests valid", "num-requests", len(manifests))

	if len(manifests) > 0 {
		// XXX: only one version means only one valid manifest
		m.manifests = append(m.manifests, manifests[0])
	}
}

func (m *manager) validateRequest(req manifestRequest) error {
	// ensure that an uploaded manifest matches the hash declared on
	// the Akash Deployment.Version
	version, err := sdl.ManifestVersion(req.value.Manifest)
	if err != nil {
		return err
	}
	if !bytes.Equal(version, m.data.Deployment.Version) {
		return ErrManifestVersion
	}

	// TODO - test this code path
	if err := validation.ValidateManifest(req.value.Manifest); err != nil {
		return err
	}

	// TODO - figure out why we pass a pointer here
	// TODO - test this code path
	if err := validation.ValidateManifestWithDeployment(&req.value.Manifest, m.data.Groups); err != nil {
		return err
	}
	return nil
}
