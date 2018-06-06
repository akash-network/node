package cluster

import (
	"fmt"

	lifecycle "github.com/boz/go-lifecycle"
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

type deploymentMonitor struct {
	client Client

	state deploymentState

	lease  types.LeaseID
	dgroup *types.DeploymentGroup
	mgroup *types.ManifestGroup

	updatech   chan *types.ManifestGroup
	teardownch chan struct{}

	log log.Logger
	lc  lifecycle.Lifecycle
}

func newDeploymentMonitor(s *service, lease types.LeaseID, dgroup *types.DeploymentGroup, mgroup *types.ManifestGroup) *deploymentMonitor {

	log := s.log.With(
		"cmp", "deployment-monitor",
		"dgroup", dgroup.DeploymentGroupID,
		"mgroup", mgroup.Name)

	dm := &deploymentMonitor{
		client:     s.client,
		state:      dsDeployActive,
		lease:      lease,
		dgroup:     dgroup,
		mgroup:     mgroup,
		updatech:   make(chan *types.ManifestGroup),
		teardownch: make(chan struct{}),
		log:        log,
		lc:         lifecycle.New(),
	}

	go dm.lc.WatchChannel(s.lc.ShuttingDown())
	go dm.run()

	go func() {
		<-dm.lc.Done()
		s.monitorch <- dm
	}()

	return dm
}

func (dm *deploymentMonitor) update(mgroup *types.ManifestGroup) error {
	select {
	case dm.updatech <- mgroup:
		return nil
	case <-dm.lc.ShuttingDown():
		return fmt.Errorf("not running")
	}
}

func (dm *deploymentMonitor) teardown() error {
	select {
	case dm.teardownch <- struct{}{}:
		return nil
	case <-dm.lc.ShuttingDown():
		return fmt.Errorf("not running")
	}
}

func (dm *deploymentMonitor) run() {
	defer dm.lc.ShutdownCompleted()

	runch := dm.do(dm.doDeploy)

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
			case dsDeployPending:
				dm.mgroup = mgroup
			case dsDeployComplete:
				dm.mgroup = mgroup

				// start update
				dm.state = dsDeployActive
				runch = dm.do(dm.doDeploy)

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

			case dsDeployPending:

				// start update
				dm.state = dsDeployActive
				runch = dm.do(dm.doDeploy)

			case dsDeployComplete:

				panic(fmt.Errorf("INVALID STATE: runch read on %v", dm.state))

			case dsTeardownActive:
				dm.state = dsTeardownComplete
				break loop

			case dsTeardownPending:

				// start teardown
				dm.state = dsTeardownActive
				runch = dm.do(dm.doTeardown)

			case dsTeardownComplete:

				panic(fmt.Errorf("INVALID STATE: runch read on %v", dm.state))

			}

		case <-dm.teardownch:
			dm.log.Debug("teardown request")

			switch dm.state {
			case dsDeployActive:

				dm.state = dsTeardownPending

			case dsDeployPending:

				dm.state = dsTeardownPending

			case dsDeployComplete:

				// start teardown
				dm.state = dsTeardownActive
				runch = dm.do(dm.doTeardown)

			case dsTeardownActive, dsTeardownPending, dsTeardownComplete:
			}
		}
	}

	if runch != nil {
		<-runch
	}
}

func (dm *deploymentMonitor) doDeploy() error {
	return dm.client.Deploy(dm.lease.OrderID(), dm.mgroup)
}

func (dm *deploymentMonitor) doTeardown() error {
	return dm.client.Teardown(dm.lease.OrderID())
}

func (dm *deploymentMonitor) do(fn func() error) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- fn()
	}()
	return ch
}
