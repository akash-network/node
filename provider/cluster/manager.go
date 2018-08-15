package cluster

import (
	"fmt"
	"sync"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
)

type deploymentState string

const (
	dsDeployActive     deploymentState = "deploy-active"
	dsDeployPending                    = "deploy-pending"
	dsDeployComplete                   = "deploy-complete"
	dsTeardownActive                   = "teardown-active"
	dsTeardownPending                  = "teardown-pending"
	dsTeardownComplete                 = "teardown-complete"
)

type deploymentManager struct {
	bus     event.Bus
	client  Client
	session session.Session

	state deploymentState

	lease  types.LeaseID
	mgroup *types.ManifestGroup

	monitor *deploymentMonitor
	wg      sync.WaitGroup

	updatech   chan *types.ManifestGroup
	teardownch chan struct{}

	log log.Logger
	lc  lifecycle.Lifecycle
}

func newDeploymentManager(s *service, lease types.LeaseID, mgroup *types.ManifestGroup) *deploymentManager {

	log := s.log.With("cmp", "deployment-manager",
		"lease", lease, "manifest-group", mgroup.Name)

	dm := &deploymentManager{
		bus:        s.bus,
		client:     s.client,
		session:    s.session,
		state:      dsDeployActive,
		lease:      lease,
		mgroup:     mgroup,
		wg:         sync.WaitGroup{},
		updatech:   make(chan *types.ManifestGroup),
		teardownch: make(chan struct{}),
		log:        log,
		lc:         lifecycle.New(),
	}

	go dm.lc.WatchChannel(s.lc.ShuttingDown())
	go dm.run()

	go func() {
		<-dm.lc.Done()
		s.managerch <- dm
	}()

	return dm
}

func (dm *deploymentManager) update(mgroup *types.ManifestGroup) error {
	select {
	case dm.updatech <- mgroup:
		return nil
	case <-dm.lc.ShuttingDown():
		return fmt.Errorf("not running")
	}
}

func (dm *deploymentManager) teardown() error {
	select {
	case dm.teardownch <- struct{}{}:
		return nil
	case <-dm.lc.ShuttingDown():
		return fmt.Errorf("not running")
	}
}

func (dm *deploymentManager) run() {
	defer dm.lc.ShutdownCompleted()

	runch := dm.startDeploy()

loop:
	for {
		select {

		case err := <-dm.lc.ShutdownRequest():
			dm.lc.ShutdownInitiated(err)
			break loop

		case mgroup := <-dm.updatech:

			dm.mgroup = mgroup

			switch dm.state {
			case dsDeployActive:
				dm.mgroup = mgroup
				dm.state = dsDeployPending
			case dsDeployPending:
				dm.mgroup = mgroup
			case dsDeployComplete:
				dm.mgroup = mgroup

				// start update
				runch = dm.startDeploy()

			case dsTeardownActive, dsTeardownPending, dsTeardownComplete:
			}

		case result := <-runch:
			runch = nil

			if result != nil {
				dm.log.Error("execution error", "state", dm.state, "err", result)
			}

			switch dm.state {
			case dsDeployActive:
				dm.log.Debug("deploy complete")
				dm.state = dsDeployComplete

				dm.startMonitor()

			case dsDeployPending:
				// start update
				runch = dm.startDeploy()

			case dsDeployComplete:

				panic(fmt.Errorf("INVALID STATE: runch read on %v", dm.state))

			case dsTeardownActive:
				dm.state = dsTeardownComplete
				break loop

			case dsTeardownPending:

				// start teardown
				runch = dm.startTeardown()

			case dsTeardownComplete:

				panic(fmt.Errorf("INVALID STATE: runch read on %v", dm.state))

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

	if runch != nil {
		<-runch
	}

	dm.wg.Wait()
}

func (dm *deploymentManager) startMonitor() {
	dm.wg.Add(1)
	dm.monitor = newDeploymentMonitor(dm)
	go func(m *deploymentMonitor) {
		defer dm.wg.Done()
		<-m.done()
	}(dm.monitor)
}

func (dm *deploymentManager) stopMonitor() {
	if dm.monitor != nil {
		dm.monitor.shutdown()
	}
}

func (dm *deploymentManager) startDeploy() <-chan error {
	dm.stopMonitor()
	dm.state = dsDeployActive
	return dm.do(dm.doDeploy)
}

func (dm *deploymentManager) startTeardown() <-chan error {
	dm.stopMonitor()
	dm.state = dsTeardownActive
	return dm.do(dm.doTeardown)
}

func (dm *deploymentManager) doDeploy() error {
	return dm.client.Deploy(dm.lease, dm.mgroup)
}

func (dm *deploymentManager) doTeardown() error {
	return dm.client.TeardownLease(dm.lease)
}

func (dm *deploymentManager) do(fn func() error) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- fn()
	}()
	return ch
}
