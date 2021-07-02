package cluster

import (
	"context"
	"errors"
	"fmt"
	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"time"

	retry "github.com/avast/retry-go"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"
	"sync"
)

type deploymentState string

const (
	dsDeployActive     deploymentState = "deploy-active"
	dsDeployPending    deploymentState = "deploy-pending"
	dsDeployComplete   deploymentState = "deploy-complete"
	dsTeardownActive   deploymentState = "teardown-active"
	dsTeardownPending  deploymentState = "teardown-pending"
	dsTeardownComplete deploymentState = "teardown-complete"
	dsTakeoverHostname deploymentState = "takeover-hostname"
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

	monitor    *deploymentMonitor
	withdrawal *deploymentWithdrawal
	wg         sync.WaitGroup

	updatech   chan *manifest.Group
	teardownch chan struct{}

	log             log.Logger
	lc              lifecycle.Lifecycle
	hostnameService HostnameServiceClient

	deploymentOkNotif chan struct{}
}

func newDeploymentManager(s *service, lease mtypes.LeaseID, mgroup *manifest.Group) *deploymentManager {
	log := s.log.With("cmp", "deployment-manager", "lease", lease, "manifest-group", mgroup.Name)

	dm := &deploymentManager{
		bus:               s.bus,
		client:            s.client,
		session:           s.session,
		state:             dsDeployActive,
		lease:             lease,
		mgroup:            mgroup,
		wg:                sync.WaitGroup{},
		updatech:          make(chan *manifest.Group),
		teardownch:        make(chan struct{}),
		log:               log,
		lc:                lifecycle.New(),
		hostnameService:   s.HostnameService(),
		deploymentOkNotif: make(chan struct{}, 1),
	}

	go dm.lc.WatchChannel(s.lc.ShuttingDown())
	go dm.run()

	go func() {
		<-dm.lc.Done()
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

func (dm *deploymentManager) run() {
	defer dm.lc.ShutdownCompleted()
	var shutdownErr error
	var hostnamesToTakeover []ReplacedHostname

	deployHostnamesCh, runch := dm.startDeploy(true)

	// TODO - update me so that a list does not have to be passed in
	defer dm.hostnameService.ReleaseHostnames(dm.lease.DeploymentID())

	var okNotify <-chan struct{}

loop:
	for {
		select {

		case shutdownErr = <-dm.lc.ShutdownRequest():
			break loop

		case mgroup := <-dm.updatech:
			dm.mgroup = mgroup

			switch dm.state {
			case dsDeployActive:
				dm.mgroup = mgroup
				dm.state = dsDeployPending
			case dsDeployPending:
				dm.mgroup = mgroup
			case dsDeployComplete, dsTakeoverHostname:
				dm.mgroup = mgroup
				// start update
				deployHostnamesCh, runch = dm.startDeploy(true)
			case dsTeardownActive, dsTeardownPending, dsTeardownComplete:
				// do nothing
			}
		case hostnamesToTakeover = <-deployHostnamesCh:
			dm.log.Info("ready to claim hostnames")
			deployHostnamesCh = nil

			// Clear the channel if a value is present
			select {
			case <-dm.deploymentOkNotif:
			default:
			}

			okNotify = dm.deploymentOkNotif

		case <-okNotify:
			okNotify = nil
			// The deploy should be running at this point. If not bail
			if dm.state != dsDeployComplete {
				continue
			}
			dm.log.Info("taking over hostnames after deployment is up", "cnt", len(hostnamesToTakeover))
			runch = dm.startTakeoverHostnames(hostnamesToTakeover)

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
				deployHostnamesCh, runch = dm.startDeploy(true)
			case dsTakeoverHostname:
				dm.log.Info("deployment finalized")
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
			case dsDeployComplete, dsTakeoverHostname:
				// start teardown
				runch = dm.startTeardown()
			case dsTeardownActive, dsTeardownPending, dsTeardownComplete:
			}
		}
	}

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

func (dm *deploymentManager) startTakeoverHostnames(changedHostnames []ReplacedHostname) <-chan error {
	dm.state = dsTakeoverHostname
	return dm.do(func() error {
		err := dm.takeoverHostnames(changedHostnames)
		if err != nil {
			return nil
		}

		_, err = dm.doDeploy(false)
		return err
	})
}

func (dm *deploymentManager) takeoverHostnames(changedHostnames []ReplacedHostname) error {
	dm.log.Info("deploy taking over hostnames", "count", len(changedHostnames))
	ownerAddr, err := dm.lease.DeploymentID().GetOwnerAddress()
	if err != nil {
		return err
	}

	ctx := context.Background() // TODO - choose a context that makes sense here
	// Strip each and every hostname from its existing deployment within kubernetes
	for _, changedHostname := range changedHostnames {
		err = dm.client.ClearHostname(ctx, ownerAddr, changedHostname.PreviousDeploymentSequence, changedHostname.Hostname)
		if err != nil {
			if !errors.Is(err, ErrClearHostnameNoMatches) {
				return err
			}

			// The hostname to be cleared was not found, this is usually due to a race condition
			dm.log.Info("hostname did not exist", "hostname", changedHostname)
		} else {
			dm.log.Info("hostname cleared", "hostname", changedHostname)
		}
	}

	return nil
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

func (dm *deploymentManager) startDeploy(reserveHostnames bool) (<-chan []ReplacedHostname, <-chan error) {
	dm.stopMonitor()
	dm.state = dsDeployActive

	chErr := make(chan error, 1)
	chHostnames := make(chan []ReplacedHostname, 1)
	go func() {
		hostnames, err := dm.doDeploy(reserveHostnames)
		if err != nil {
			chErr <- err
			return
		}

		chErr <- nil

		if len(hostnames) != 0 {
			chHostnames <- hostnames
		}
	}()

	return chHostnames, chErr
}

func (dm *deploymentManager) startTeardown() <-chan error {
	dm.stopMonitor()
	dm.state = dsTeardownActive
	return dm.do(dm.doTeardown)
}

func (dm *deploymentManager) doDeploy(reserveHostnames bool) ([]ReplacedHostname, error) {
	var changedHostnames []ReplacedHostname
	var err error
	if reserveHostnames {
		allHostnames := util.AllHostnamesOfManifestGroup(*dm.mgroup)
		// Either reserve the hostnames, or confirm that they already are held
		reservationResult := dm.hostnameService.ReserveHostnames(allHostnames, dm.lease.DeploymentID())
		changedHostnames, err = reservationResult.Wait(dm.lc.ShuttingDown())
		if err != nil {
			deploymentCounter.WithLabelValues("reserve-hostnames", "err").Inc()
			dm.log.Error("deploy hostname reservation error", "state", dm.state, "err", err)
			return nil, err
		}
		deploymentCounter.WithLabelValues("reserve-hostnames", "success").Inc()
	}

	// Don't use a context tied to the lifecycle, as we don't want to cancel Kubernetes operations
	ctx := context.Background()

	holdHostnames := make([]string, len(changedHostnames))
	for i, v := range changedHostnames {
		holdHostnames[i] = v.Hostname
	}
	err = dm.client.Deploy(ctx, dm.lease, dm.mgroup, holdHostnames)
	label := "success"
	if err != nil {
		label = "fail"
	}
	deploymentCounter.WithLabelValues("deploy", label).Inc()
	return changedHostnames, err

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
	return result
}

func (dm *deploymentManager) do(fn func() error) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- fn()
	}()
	return ch
}
