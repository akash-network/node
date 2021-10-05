package manifest

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	clustertypes "github.com/ovrclk/akash/provider/cluster/types"
	"time"

	"github.com/ovrclk/akash/provider/cluster/util"

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
	ErrManifestVersion         = errors.New("manifest version validation failed")
	ErrNoManifestForDeployment = errors.New("manifest not yet received for that deployment")
	ErrNoLeaseForDeployment    = errors.New("no lease for deployment")
)

func newManager(h *service, daddr dtypes.DeploymentID) *manager {
	session := h.session.ForModule("manifest-manager")

	m := &manager{
		daddr:           daddr,
		session:         session,
		bus:             h.bus,
		leasech:         make(chan event.LeaseWon),
		rmleasech:       make(chan mtypes.LeaseID),
		manifestch:      make(chan manifestRequest),
		updatech:        make(chan []byte),
		log:             session.Log().With("deployment", daddr),
		lc:              lifecycle.New(),
		config:          h.config,
		hostnameService: h.hostnameService,
	}

	go m.lc.WatchChannel(h.lc.ShuttingDown())
	go m.run(h.managerch)

	return m
}

// 'manager' facilitates operations around a configured Deployment.
type manager struct {
	config  ServiceConfig
	daddr   dtypes.DeploymentID
	session session.Session
	bus     pubsub.Bus

	leasech    chan event.LeaseWon
	rmleasech  chan mtypes.LeaseID
	manifestch chan manifestRequest
	manifestRequestTimeOut chan manifestRequest
	updatech   chan []byte

	data            *dtypes.QueryDeploymentResponse
	requests        []manifestRequest
	pendingRequests []manifestRequest
	pendingContext context.Context
	pendingContextCancel context.CancelFunc
	manifests       []*manifest.Manifest
	versions        [][]byte

	localLeases []event.LeaseWon
	localLeaseAt time.Time

	stoptimer *time.Timer

	log log.Logger
	lc  lifecycle.Lifecycle

	hostnameService clustertypes.HostnameServiceClient
}

func (m *manager) getPendingContext() context.Context{
	if m.pendingContextCancel == nil {
		m.pendingContext, m.pendingContextCancel = context.WithCancel(context.Background())
	}
	return m.pendingContext
}

func (m *manager) cancelPendingContext() {
	if nil != m.pendingContextCancel {
		m.pendingContextCancel()
		m.pendingContextCancel = nil
		m.pendingContext = nil
	}
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
		m.log.Error("not running: version update", "version", hex.EncodeToString(version))
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
			m.localLeaseAt = time.Time{}

			m.emitReceivedEvents()
			m.maybeScheduleStop()
			runch = m.maybeFetchData(ctx, runch)

		case id := <-m.rmleasech:
			m.log.Info("lease removed", "lease", id)
			m.localLeaseAt = time.Time{}
			m.maybeScheduleStop()

		case req := <-m.manifestch:
			m.log.Info("manifest received")

			m.requests = append(m.requests, req)
			m.validateRequests()
			m.emitReceivedEvents()
			m.maybeScheduleStop()
			runch = m.maybeFetchData(ctx, runch)

		case version := <-m.updatech:
			m.log.Info("received version", "version", hex.EncodeToString(version))
			m.versions = append(m.versions, version)
			if m.data != nil {
				m.data.Deployment.Version = version
			}

		case result := <-runch:
			runch = nil

			if err := result.Error(); err != nil {
				m.log.Error("error fetching data", "err", err)
				// Fetching data failed, all requests are now in an error state
				m.fillAllRequests(err)
				break
			}

			m.data = result.Value().(*dtypes.QueryDeploymentResponse)

			m.log.Info("data received", "version", hex.EncodeToString(m.data.Deployment.Version))

			m.validateRequests()
			m.emitReceivedEvents()
			m.maybeScheduleStop()

		case timedOutRequest := <- m.manifestRequestTimeOut:
			m.handleTimeout(timedOutRequest)
		}
	}

	cancel()

	m.fillAllRequests(ErrNotRunning)

	if m.stoptimer != nil {
		if m.stoptimer.Stop() {
			<-m.stoptimer.C
		}
	}

	if runch != nil {
		<-runch
	}

}

func (m *manager) handleTimeout(req manifestRequest){
	x := -1
	for i, reqEntry := range m.pendingRequests {
		if reqEntry == req {
			x = i
			break
		}
	}

	if x != -1 {
		removed := m.requests[x]
		replacement := make([]manifestRequest, len(m.requests) - 1)
		i := 0
		for j, reqEntry := range m.pendingRequests {
			if j != x {
				replacement[i] = reqEntry
				i++
			}
		}
		removed.ch <- removed.ctx.Err()
		m.pendingRequests = replacement
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

func (m *manager) doFetchData(ctx context.Context) (*dtypes.QueryDeploymentResponse, error) {
	res, err := m.session.Client().Query().Deployment(ctx, &dtypes.QueryDeploymentRequest{ID: m.daddr})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (m *manager) maybeScheduleStop() bool { // nolint:golint,unparam
	leases, err := m.getLeases()
	if err != nil {
		m.log.Error("failed getting leases", "err", err)
	}
	if len(leases) > 0 || len(m.manifests) > 0 {
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
		const manifestLingerDuration = time.Minute * time.Duration(5)
		m.log.Info("starting stop timer", "duration", manifestLingerDuration)
		m.stoptimer = time.NewTimer(manifestLingerDuration)
	}
	return true
}

func (m *manager) fillAllRequests(response error) {
	m.cancelPendingContext()
	for _, req := range m.pendingRequests {
		req.ch <- response
	}
	m.pendingRequests = nil

	for _, req := range m.requests {
		req.ch <- response
	}
	m.requests = nil
}

var errNoGroupForLease = errors.New("group not found")

func (m *manager) getLeases() ([]event.LeaseWon, error) {
	const maxResultAge = time.Second * 5
	elapsed := time.Now().Sub(m.localLeaseAt)
	if elapsed < maxResultAge { // Check to see if the cached result can be used
		return m.localLeases, nil
	}
	deployment, err := m.session.Client().Query().Deployment(context.Background(), &dtypes.QueryDeploymentRequest{
		ID: m.daddr,
	})

	if err != nil {
		return nil, err
	}

	response, err := m.session.Client().Query().Leases(context.Background(), &mtypes.QueryLeasesRequest{
		Filters:    mtypes.LeaseFilters{
			Owner:    m.daddr.Owner,
			DSeq:     m.daddr.DSeq,
			GSeq:     0,
			OSeq:     0,
			Provider: m.session.Provider().GetOwner(),
			State:    mtypes.LeaseActive.String(),
		},
		Pagination: nil,
	})

	if err != nil {
		return nil, err
	}

	groups := make(map[uint32]dtypes.Group)
	for _, g := range deployment.GetGroups() {
		groups[g.ID().GSeq] = g
	}

	leases := make([]event.LeaseWon, len(response.Leases))
	for i, leaseEntry := range response.Leases {
		lease := leaseEntry.GetLease()
		leaseID := lease.GetLeaseID()
		groupForLease, foundGroup := groups[leaseID.GetGSeq()]
		if !foundGroup {
			return nil, fmt.Errorf("%w: could not locate group %v ", errNoGroupForLease, leaseID)
		}
		ev := event.LeaseWon{
			LeaseID: leaseID,
			Group:   &groupForLease,
			Price:   lease.GetPrice(),
		}

		leases[i] = ev
	}
	m.localLeases = leases
	m.localLeaseAt = time.Now()
	return m.localLeases, nil
}

func (m *manager) emitReceivedEvents()  {
	leases, err := m.getLeases()
	if err != nil {
		m.fillAllRequests(err)
		return
	}

	if len(leases) == 0 {
		m.log.Debug("emit received events skips due to no leases", "data", m.data, "leases", len(leases), "manifests", len(m.manifests))
		m.fillAllRequests(ErrNoLeaseForDeployment)
		return
	}
	if m.data == nil || len(m.manifests) == 0 {
		m.log.Debug("emit received events skipped", "data", m.data, "leases", len(leases), "manifests", len(m.manifests))
		return
	}

	latestManifest := m.manifests[len(m.manifests)-1]
	m.log.Debug("publishing manifest received", "num-leases", len(leases))
	for _, lease := range leases {
		m.log.Debug("publishing manifest received for lease", "lease_id", lease.LeaseID)
		if err := m.bus.Publish(event.ManifestReceived{
			LeaseID:    lease.LeaseID,
			Group:      lease.Group,
			Manifest:   latestManifest,
			Deployment: m.data,
		}); err != nil {
			m.log.Error("publishing event", "err", err, "lease", lease.LeaseID)
		}
	}

	// A manifest has been published, satisfy all pending requests
	m.cancelPendingContext()
	for _, req := range m.pendingRequests {
		req.ch <- nil
	}
	m.pendingRequests = nil
}


func handleManifestRequestTimeout(ctx context.Context, mr manifestRequest, lc lifecycle.Lifecycle, timedOut chan <- manifestRequest) {
	select {
	case <- ctx.Done(): // pending requests are complete
		return
	case <-mr.ctx.Done(): // request context expires
		timedOut <- mr // Tell manager timeout happened
	case <-lc.ShuttingDown(): // manager shuts down
		return
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

		// The manifest has been grabbed from the request but not published yet, store this response
		m.pendingRequests = append(m.pendingRequests, req)

		go handleManifestRequestTimeout(m.getPendingContext(), req, m.lc, m.manifestRequestTimeOut)
	}
	m.requests = nil // all requests processed at this time

	m.log.Debug("requests valid", "num-requests", len(manifests))

	if len(manifests) > 0 {
		// XXX: only one version means only one valid manifest
		m.manifests = append(m.manifests, manifests[0])
	}
}

var errManifestRejected = errors.New("manifest rejected")

func (m *manager) checkHostnamesForManifest(requestManifest manifest.Manifest, groupNames []string) error {
	// Check if the hostnames are available. Do not block forever
	ownerAddr, err := m.data.GetDeployment().DeploymentID.GetOwnerAddress()
	if err != nil {
		return err
	}

	allHostnames := make([]string, 0)

	for _, mgroup := range requestManifest.GetGroups() {
		for _, groupName := range groupNames {
			// Only check leases with a matching deployment ID & group name
			if groupName != mgroup.GetName() {
				continue
			}

			allHostnames = append(allHostnames, util.AllHostnamesOfManifestGroup(mgroup)...)
			if !m.config.HTTPServicesRequireAtLeastOneHost {
				continue
			}
			// For each service that exposes via an Ingress, then require a hsotname
			for _, service := range mgroup.Services {
				for _, expose := range service.Expose {
					if util.ShouldBeIngress(expose) && len(expose.Hosts) == 0 {
						return fmt.Errorf("%w: service %q exposed on %d:%s must have a hostname", errManifestRejected, service.Name, util.ExposeExternalPort(expose), expose.Proto)
					}
				}
			}
		}
	}

	return m.hostnameService.CanReserveHostnames(allHostnames, ownerAddr)
}

func (m *manager) validateRequest(req manifestRequest) error {
	// ensure that an uploaded manifest matches the hash declared on
	// the Akash Deployment.Version
	version, err := sdl.ManifestVersion(req.value.Manifest)
	if err != nil {
		return err
	}

	var versionExpected []byte

	if len(m.versions) != 0 {
		versionExpected = m.versions[len(m.versions)-1]
	} else {
		versionExpected = m.data.Deployment.Version
	}
	if !bytes.Equal(version, versionExpected) {
		m.log.Info("deployment version mismatch", "expected", m.data.Deployment.Version, "got", version)
		return ErrManifestVersion
	}

	if err := validation.ValidateManifest(req.value.Manifest); err != nil {
		return err
	}

	if err := validation.ValidateManifestWithDeployment(&req.value.Manifest, m.data.Groups); err != nil {
		return err
	}

	groupNames := make([]string, 0)
	leases, err := m.getLeases()
	if err != nil {
		return err
	}
	for _, lease := range leases {
		groupNames = append(groupNames, lease.Group.GroupSpec.Name)
	}
	// Check that hostnames are not in use
	if err := m.checkHostnamesForManifest(req.value.Manifest, groupNames); err != nil {
		return err
	}

	return nil
}
