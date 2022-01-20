package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/boz/go-lifecycle"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	clustertypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/provider/event"

	"github.com/avast/retry-go"
	"github.com/tendermint/tendermint/libs/log"

	clusterutil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"

	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type deploymentState string

const (
	dsDeployActive     deploymentState = "deploy-active"
	dsDeployPending    deploymentState = "deploy-pending"
	dsDeployComplete   deploymentState = "deploy-complete"
	dsTeardownActive   deploymentState = "teardown-active"
	dsTeardownPending  deploymentState = "teardown-pending"
	dsTeardownComplete deploymentState = "teardown-complete"
)

var (
	deploymentCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_deployment",
	}, []string{"action", "result"})

	monitorCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_deployment_monitor",
	}, []string{"action"})
)

type deploymentManager struct {
	bus     pubsub.Bus
	client  Client
	session session.Session

	state deploymentState

	lease  mtypes.LeaseID
	mgroup *manifest.Group

	monitor          *deploymentMonitor
	withdrawal       *deploymentWithdrawal
	wg               sync.WaitGroup
	updatech         chan *manifest.Group
	teardownch       chan struct{}
	currentHostnames map[string]struct{}
	currentIPs       map[string]serviceExposeWithServiceName

	log             log.Logger
	lc              lifecycle.Lifecycle
	hostnameService clustertypes.HostnameServiceClient

	config Config
}

func newDeploymentManager(s *service, lease mtypes.LeaseID, mgroup *manifest.Group) *deploymentManager {
	logger := s.log.With("cmp", "deployment-manager", "lease", lease, "manifest-group", mgroup.Name)

	dm := &deploymentManager{
		bus:              s.bus,
		client:           s.client,
		session:          s.session,
		state:            dsDeployActive,
		lease:            lease,
		mgroup:           mgroup,
		wg:               sync.WaitGroup{},
		updatech:         make(chan *manifest.Group),
		teardownch:       make(chan struct{}),
		log:              logger,
		lc:               lifecycle.New(),
		hostnameService:  s.HostnameService(),
		config:           s.config,
		currentHostnames: make(map[string]struct{}),
		currentIPs:       make(map[string]serviceExposeWithServiceName),
	}

	ctx, _ := TieContextToLifecycle(context.Background(), s.lc)

	go dm.run(ctx)

	go func() {
		<-dm.lc.Done()
		dm.log.Debug("sending manager into channel")
		s.managerch <- dm
	}()

	return dm
}

func (dm *deploymentManager) update(mgroup *manifest.Group) error {
	select {
	case dm.updatech <- mgroup:
		return nil
	case <-dm.lc.ShuttingDown():
		return ErrNotRunning
	}
}

func (dm *deploymentManager) teardown() error {
	select {
	case dm.teardownch <- struct{}{}:
		return nil
	case <-dm.lc.ShuttingDown():
		return ErrNotRunning
	}
}

func (dm *deploymentManager) handleUpdate(ctx context.Context) <-chan error {
	switch dm.state {
	case dsDeployActive:
		dm.state = dsDeployPending
	case dsDeployComplete:
		// start update
		return dm.startDeploy(ctx)
	case dsDeployPending, dsTeardownActive, dsTeardownPending, dsTeardownComplete:
		// do nothing
	}

	return nil
}

func (dm *deploymentManager) run(ctx context.Context) {
	defer dm.lc.ShutdownCompleted()
	var shutdownErr error

	runch := dm.startDeploy(ctx)

	defer func() {
		err := dm.hostnameService.ReleaseHostnames(dm.lease)
		if err != nil {
			dm.log.Error("failed releasing hostnames", "err", err)
		}
		dm.log.Debug("hostnames released")
	}()
	var teardownErr error
loop:
	for {
		select {

		case shutdownErr = <-dm.lc.ShutdownRequest():
			break loop

		case mgroup := <-dm.updatech:
			dm.mgroup = mgroup
			newch := dm.handleUpdate(ctx)
			if newch != nil {
				runch = newch
			}

		case result := <-runch:
			runch = nil
			if result != nil {
				dm.log.Error("execution error", "state", dm.state, "err", result)
			}
			switch dm.state {
			case dsDeployActive:
				if result != nil {
					// Run the teardown code to get rid of anything created that might be hanging out
					runch = dm.startTeardown()

				} else {
					dm.log.Debug("deploy complete")
					dm.state = dsDeployComplete
					dm.startMonitor()
					dm.startWithdrawal()
				}
			case dsDeployPending:
				if result != nil {
					break loop
				}
				// start update
				runch = dm.startDeploy(ctx)
			case dsDeployComplete:
				panic(fmt.Sprintf("INVALID STATE: runch read on %v", dm.state))
			case dsTeardownActive:
				teardownErr = result
				dm.state = dsTeardownComplete
				dm.log.Debug("teardown complete")
				break loop
			case dsTeardownPending:
				// start teardown
				runch = dm.startTeardown()
			case dsTeardownComplete:
				panic(fmt.Sprintf("INVALID STATE: runch read on %v", dm.state))
			}

		case <-dm.teardownch:
			dm.log.Debug("teardown request")
			dm.stopMonitor()
			switch dm.state {
			case dsDeployActive:
				dm.state = dsTeardownPending
			case dsDeployPending:
				dm.state = dsTeardownPending
			case dsDeployComplete:
				// start teardown
				runch = dm.startTeardown()
			case dsTeardownActive, dsTeardownPending, dsTeardownComplete:
			}
		}
	}

	dm.log.Debug("shutting down")
	dm.lc.ShutdownInitiated(shutdownErr)
	if runch != nil {
		<-runch
		dm.log.Debug("read from runch during shutdown")
	}

	dm.log.Debug("waiting on dm.wg")
	dm.wg.Wait()

	if nil != dm.withdrawal {
		dm.log.Debug("waiting on withdrawal")
		dm.withdrawal.lc.Shutdown(nil)
	}
	dm.log.Info("shutdown complete")

	if dm.state != dsTeardownComplete {
		dm.log.Info("shutting down unclean, running teardown now")
		const uncleanShutdownGracePeriod = 30 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), uncleanShutdownGracePeriod)
		defer cancel()
		teardownErr = dm.doTeardown(ctx)
	}

	if teardownErr != nil {
		// TODO - report this to an external service
		dm.log.Error("lease teardwon failed", "err", teardownErr)
	}
}

func (dm *deploymentManager) startWithdrawal() {
	dm.wg.Add(1)
	dm.withdrawal = newDeploymentWithdrawal(dm)
	go func(m *deploymentMonitor) {
		defer dm.wg.Done()
		<-m.done()
	}(dm.monitor)
}

func (dm *deploymentManager) startMonitor() {
	dm.wg.Add(1)
	dm.monitor = newDeploymentMonitor(dm)
	monitorCounter.WithLabelValues("start").Inc()
	go func(m *deploymentMonitor) {
		defer dm.wg.Done()
		<-m.done()
	}(dm.monitor)
}

func (dm *deploymentManager) stopMonitor() {
	if dm.monitor != nil {
		monitorCounter.WithLabelValues("stop").Inc()
		dm.monitor.shutdown()
	}
}

func (dm *deploymentManager) startDeploy(ctx context.Context) <-chan error {
	dm.stopMonitor()
	dm.state = dsDeployActive

	chErr := make(chan error, 1)

	go func() {
		hostnames, err := dm.doDeploy(ctx)
		if err != nil {
			chErr <- err
			return
		}

		if len(hostnames) != 0 {
			// start update to takeover hostnames
			dm.log.Info("hostnames withheld from deployment", "cnt", len(hostnames))
		}

		groupCopy := *dm.mgroup
		ev := event.ClusterDeployment{
			LeaseID: dm.lease,
			Group:   &groupCopy,
			Status:  event.ClusterDeploymentUpdated,
		}
		err = dm.bus.Publish(ev)
		if err != nil {
			dm.log.Error("failed publishing event", "err", err)
		}

		close(chErr)
	}()
	return chErr
}

func (dm *deploymentManager) startTeardown() <-chan error {
	dm.stopMonitor()
	dm.state = dsTeardownActive
	return dm.do(func() error {
		// Don't use a context tied to the lifecycle, as we don't want to cancel Kubernetes operations
		return dm.doTeardown(context.Background())
	})
}

type serviceExposeWithServiceName struct {
	expose manifest.ServiceExpose
	name   string
}

func (sewsn serviceExposeWithServiceName) idIP() string {
	return fmt.Sprintf("%s-%s-%d-%v", sewsn.name, sewsn.expose.IP, sewsn.expose.Port, sewsn.expose.Proto)
}

func (dm *deploymentManager) doDeploy(ctx context.Context) ([]string, error) {
	allHostnames := util.AllHostnamesOfManifestGroup(*dm.mgroup)
	// Either reserve the hostnames, or confirm that they already are held
	withheldHostnames, err := dm.hostnameService.ReserveHostnames(ctx, allHostnames, dm.lease)

	if err != nil {
		deploymentCounter.WithLabelValues("reserve-hostnames", "err").Inc()
		dm.log.Error("deploy hostname reservation error", "state", dm.state, "err", err)
		return nil, err
	}
	deploymentCounter.WithLabelValues("reserve-hostnames", "success").Inc()

	dm.log.Info("hostnames withheld", "cnt", len(withheldHostnames))

	hostnamesInThisRequest := make(map[string]struct{})
	for _, hostname := range allHostnames {
		hostnamesInThisRequest[hostname] = struct{}{}
	}

	// Figure out what hostnames were removed from the manifest if any
	purgeHostnames := make([]string, 0)
	for hostnameInUse := range dm.currentHostnames {
		_, stillInUse := hostnamesInThisRequest[hostnameInUse]
		if !stillInUse {
			purgeHostnames = append(purgeHostnames, hostnameInUse)
		}
	}

	// Don't use a context tied to the lifecycle, as we don't want to cancel Kubernetes operations
	deployCtx := util.ApplyToContext(context.Background(), dm.config.ClusterSettings)

	err = dm.client.Deploy(deployCtx, dm.lease, dm.mgroup)
	label := "success"
	if err != nil {
		label = "fail"
	}
	deploymentCounter.WithLabelValues("deploy", label).Inc()

	// Figure out what hostnames to declare
	blockedHostnames := make(map[string]struct{})
	for _, hostname := range withheldHostnames {
		blockedHostnames[hostname] = struct{}{}
	}
	hosts := make(map[string]manifest.ServiceExpose)
	leasedIPs := make([]serviceExposeWithServiceName, 0)
	hostToServiceName := make(map[string]string)
	ipsInThisRequest := make(map[string]serviceExposeWithServiceName)
	// clear this out so it gets repopulated
	dm.currentHostnames = make(map[string]struct{})
	// Iterate over each entry, extracting the ingress services & leased IPs
	for _, service := range dm.mgroup.Services {
		for _, expose := range service.Expose {
			if util.ShouldBeIngress(expose) {
				if dm.config.DeploymentIngressStaticHosts {
					uid := clusterutil.IngressHost(dm.lease, service.Name)
					host := fmt.Sprintf("%s.%s", uid, dm.config.DeploymentIngressDomain)
					hosts[host] = expose
					hostToServiceName[host] = service.Name
				}

				for _, host := range expose.Hosts {
					_, blocked := blockedHostnames[host]
					if !blocked {
						dm.currentHostnames[host] = struct{}{}
						hosts[host] = expose
						hostToServiceName[host] = service.Name
					}
				}
			}

			if expose.Global && len(expose.IP) != 0 {
				v := serviceExposeWithServiceName{expose: expose, name: service.Name}
				leasedIPs = append(leasedIPs, v)
				ipsInThisRequest[v.idIP()] = v
				dm.log.Debug("added IP declaration", "service", v.name, "port", v.expose.ExternalPort, "endpoint", v.expose.IP)
			}
		}
	}
	purgeIPs := make([]serviceExposeWithServiceName, 0)
	for currentIP := range dm.currentIPs {
		_, stillInUse := ipsInThisRequest[currentIP]
		if !stillInUse {
			v := dm.currentIPs[currentIP]
			purgeIPs = append(purgeIPs, v)
		}
	}

	for host, serviceExpose := range hosts {
		externalPort := uint32(util.ExposeExternalPort(serviceExpose))
		err = dm.client.DeclareHostname(ctx, dm.lease, host, hostToServiceName[host], externalPort)
		if err != nil {
			// TODO - counter
			return withheldHostnames, err
		}
	}
	// TODO - counter
	for _, hostname := range purgeHostnames {
		err = dm.client.PurgeDeclaredHostname(ctx, dm.lease, hostname)
		if err != nil {
			return withheldHostnames, err
		}
	}

	makeIPSharingLKey := func(lID mtypes.LeaseID, name string) string {
		allowedRegex := regexp.MustCompile(`[a-z,0-9,\-]+`)
		effectiveName := name
		if !allowedRegex.MatchString(name) {
			h := sha256.New()
			_, err = io.WriteString(h, name)
			if err != nil {
				panic(err)

			}
			effectiveName = strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil)[0:15]))
		}
		return fmt.Sprintf("%s-ip-%s", lID.String(), effectiveName)
	}

	for _, serviceExpose := range leasedIPs {
		endpointName := serviceExpose.expose.IP
		sharingKey := makeIPSharingLKey(dm.lease, endpointName)

		externalPort := clusterutil.ExposeExternalPort(serviceExpose.expose)
		port := serviceExpose.expose.Port

		err = dm.client.DeclareIP(ctx, dm.lease, serviceExpose.name, uint32(port), uint32(externalPort), serviceExpose.expose.Proto, sharingKey)

		if err != nil {
			return withheldHostnames, err
		}
		dm.currentIPs[serviceExpose.idIP()] = serviceExpose
	}

	// Remove old IPs not in use
	for _, serviceExpose := range purgeIPs {
		err = dm.client.PurgeDeclaredIP(ctx, dm.lease, serviceExpose.name, uint32(serviceExpose.expose.Port), serviceExpose.expose.Proto)
		if err != nil {
			return withheldHostnames, err
		}
	}

	return withheldHostnames, nil
}

func (dm *deploymentManager) getCleanupRetryOpts(ctx context.Context) []retry.Option {
	retryFn := func(err error) bool {
		isCanceled := errors.Is(err, context.Canceled)
		isDeadlineExceeeded := errors.Is(err, context.DeadlineExceeded)
		return !isCanceled && !isDeadlineExceeeded
	}
	return []retry.Option{
		retry.Attempts(50),
		retry.Delay(100 * time.Millisecond),
		retry.MaxDelay(3000 * time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
		retry.RetryIf(retryFn),
		retry.Context(ctx),
	}
}

func (dm *deploymentManager) doTeardown(ctx context.Context) error {
	const teardownActivityCount = 3
	teardownResults := make(chan error, teardownActivityCount)

	go func() {
		result := retry.Do(func() error {
			err := dm.client.TeardownLease(ctx, dm.lease)
			if err != nil {
				dm.log.Error("lease teardown failed", "err", err)
			}
			return err
		}, dm.getCleanupRetryOpts(ctx)...)

		label := "success"
		if result != nil {
			label = "fail"
		}
		deploymentCounter.WithLabelValues("teardown", label).Inc()
		teardownResults <- result
	}()

	go func() {
		result := retry.Do(func() error {
			err := dm.client.PurgeDeclaredHostnames(ctx, dm.lease)
			if err != nil {
				dm.log.Error("purge declared hostname failure", "err", err)
			}
			return err
		}, dm.getCleanupRetryOpts(ctx)...)
		// TODO - counter

		if result == nil {
			dm.log.Debug("purged hostnames")
		}
		teardownResults <- result
	}()

	go func() {
		result := retry.Do(func() error {
			err := dm.client.PurgeDeclaredIPs(ctx, dm.lease)
			if err != nil {
				dm.log.Error("purge declared ips failure", "err", err)
			}
			return err
		}, dm.getCleanupRetryOpts(ctx)...)
		// TODO - counter

		if result == nil {
			dm.log.Debug("purged ips")
		}
		teardownResults <- result
	}()

	var firstError error
	for i := 0; i != teardownActivityCount; i++ {
		select {
		case err := <-teardownResults:
			if err != nil && firstError == nil {
				firstError = err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return firstError
}

func (dm *deploymentManager) do(fn func() error) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- fn()
	}()
	return ch
}

func TieContextToLifecycle(parentCtx context.Context, lc lifecycle.Lifecycle) (context.Context, context.CancelFunc) {
	return TieContextToChannel(parentCtx, lc.ShuttingDown())
}

func TieContextToChannel(parentCtx context.Context, donech <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parentCtx)

	go func() {
		select {
		case <-donech:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
