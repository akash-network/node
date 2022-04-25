package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"errors"

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

	ErrLeaseInactive = errors.New("Inactive Lease")
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

	log             log.Logger
	lc              lifecycle.Lifecycle
	hostnameService clustertypes.HostnameServiceClient

	config Config

	serviceShuttingDown <-chan struct{}
}

func newDeploymentManager(s *service, lease mtypes.LeaseID, mgroup *manifest.Group) *deploymentManager {
	log := s.log.With("cmp", "deployment-manager", "lease", lease, "manifest-group", mgroup.Name)

	dm := &deploymentManager{
		bus:                 s.bus,
		client:              s.client,
		session:             s.session,
		state:               dsDeployActive,
		lease:               lease,
		mgroup:              mgroup,
		wg:                  sync.WaitGroup{},
		updatech:            make(chan *manifest.Group),
		teardownch:          make(chan struct{}),
		log:                 log,
		lc:                  lifecycle.New(),
		hostnameService:     s.HostnameService(),
		config:              s.config,
		serviceShuttingDown: s.lc.ShuttingDown(),
		currentHostnames:    make(map[string]struct{}),
	}

	go dm.lc.WatchChannel(dm.serviceShuttingDown)
	go dm.run()

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

func (dm *deploymentManager) handleUpdate() <-chan error {
	switch dm.state {
	case dsDeployActive:
		dm.state = dsDeployPending
	case dsDeployComplete:
		// start update
		return dm.startDeploy()
	case dsDeployPending, dsTeardownActive, dsTeardownPending, dsTeardownComplete:
		// do nothing
	}

	return nil
}

func (dm *deploymentManager) run() {
	defer dm.lc.ShutdownCompleted()
	var shutdownErr error

	runch := dm.startDeploy()

	defer func() {
		err := dm.hostnameService.ReleaseHostnames(dm.lease)
		if err != nil {
			dm.log.Error("failed releasing hostnames", "err", err)
		}
		dm.log.Debug("hostnames released")
	}()

loop:
	for {
		select {

		case shutdownErr = <-dm.lc.ShutdownRequest():
			break loop

		case mgroup := <-dm.updatech:
			dm.mgroup = mgroup
			newch := dm.handleUpdate()
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
					break loop
				}
				dm.log.Debug("deploy complete")
				dm.state = dsDeployComplete
				dm.startMonitor()
				dm.startWithdrawal()
			case dsDeployPending:
				if result != nil {
					break loop
				}
				// start update
				runch = dm.startDeploy()
			case dsDeployComplete:
				panic(fmt.Sprintf("INVALID STATE: runch read on %v", dm.state))
			case dsTeardownActive:
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

func (dm *deploymentManager) startDeploy() <-chan error {
	dm.stopMonitor()
	dm.state = dsDeployActive

	chErr := make(chan error, 1)

	go func() {
		hostnames, err := dm.doDeploy()
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
	return dm.do(dm.doTeardown)
}

func (dm *deploymentManager) doDeploy() ([]string, error) {
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Weird hack to tie this context to the lifecycle of the parent service, so this doesn't
	// block forever or anything like that
	go func() {
		select {
		case <-dm.serviceShuttingDown:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err = dm.checkLeaseActive(ctx); err != nil {
		return nil, err
	}

	// Either reserve the hostnames, or confirm that they already are held

	allHostnames := util.AllHostnamesOfManifestGroup(*dm.mgroup)
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
	hostToServiceName := make(map[string]string)

	// clear this out so it gets rebpopulated
	dm.currentHostnames = make(map[string]struct{})
	for _, service := range dm.mgroup.Services {
		for _, expose := range service.Expose {
			if !util.ShouldBeIngress(expose) {
				continue
			}

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

	return withheldHostnames, nil
}

func (dm *deploymentManager) doTeardown() error {
	// Don't use a context tied to the lifecycle, as we don't want to cancel Kubernetes operations
	ctx := context.Background()

	result := retry.Do(func() error {
		err := dm.client.TeardownLease(ctx, dm.lease)
		if err != nil {
			dm.log.Error("lease teardown failed", "err", err)
		}
		return err
	},
		retry.Attempts(50),
		retry.Delay(100*time.Millisecond),
		retry.MaxDelay(3000*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true))

	label := "success"
	if result != nil {
		label = "fail"
	}
	deploymentCounter.WithLabelValues("teardown", label).Inc()

	result = retry.Do(func() error {
		err := dm.client.PurgeDeclaredHostnames(ctx, dm.lease)
		if err != nil {
			dm.log.Error("lease teardown failed", "err", err)
		}
		return err
	},
		retry.Attempts(50),
		retry.Delay(100*time.Millisecond),
		retry.MaxDelay(3000*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true))

	// TODO - counter
	return result
}

func (dm *deploymentManager) checkLeaseActive(ctx context.Context) error {

	var lease *mtypes.QueryLeaseResponse

	err := retry.Do(func() error {
		var err error
		lease, err = dm.session.Client().Query().Lease(ctx, &mtypes.QueryLeaseRequest{
			ID: dm.lease,
		})
		if err != nil {
			dm.log.Error("lease query failed", "err")
		}
		return err
	},
		retry.Attempts(50),
		retry.Delay(100*time.Millisecond),
		retry.MaxDelay(3000*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true))

	if err != nil {
		return err
	}

	if lease.GetLease().State != mtypes.LeaseActive {
		dm.log.Error("lease not active, not deploying")
		return fmt.Errorf("%w: %s", ErrLeaseInactive, dm.lease)
	}

	return nil
}

func (dm *deploymentManager) do(fn func() error) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- fn()
	}()
	return ch
}
