package cluster

import (
	"context"
	"sync"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"
)

type stateFunc func(*deployState) stateFunc

// deployState manages the a Lease's ManifestGroup being deployed
// to a provider's client.
type deployState struct {
	ctx   context.Context
	state stateFunc
	mux   sync.Mutex

	// Unused fields
	bus     pubsub.Bus
	session session.Session

	client  Client
	lease   mtypes.LeaseID
	mgroup  *manifest.Group
	monitor *deploymentMonitor

	updatech   chan *manifest.Group
	teardownch chan struct{}
	errch      chan error

	log log.Logger
}

func newDeployState(ctx context.Context, s *service, lease mtypes.LeaseID, mgroup *manifest.Group) *deployState {
	log := s.log.With("cmp", "deployment-state", "lease", lease, "manifest-group", mgroup.Name)

	ds := &deployState{
		ctx:     ctx,
		bus:     s.bus, // unused
		client:  s.client,
		session: s.session, // unused

		state:  stateDeploying, // initial state, deploy the manifest
		lease:  lease,
		mgroup: mgroup,

		updatech:   make(chan *manifest.Group),
		teardownch: make(chan struct{}),
		errch:      nil,

		log: log.With("cmp", "deploystate"),
	}
	go runLoop(ds, stateDeploying, log)
	return ds
}

func newTestDeployState(ctx context.Context, s *service, lease mtypes.LeaseID, mgroup *manifest.Group) (*deployState, stateFunc) {
	log := s.log.With("cmp", "deployment-state", "lease", lease, "manifest-group", mgroup.Name)

	ds := &deployState{
		ctx:     ctx,
		bus:     s.bus, // unused
		client:  s.client,
		session: s.session, // unused

		state:  stateDeploying, // initial state, deploy the manifest
		lease:  lease,
		mgroup: mgroup,

		updatech:   make(chan *manifest.Group),
		teardownch: make(chan struct{}),
		errch:      nil,

		log: log.With("cmp", "deploystate"),
	}
	var sf stateFunc
	initState := stateDeploying
	go func(initState stateFunc) {
		state := initState
		for state != nil {
			sf = state(ds)
			log.With("state", s).Debug("state transitioned")
			state = sf
		}
	}(initState) // runLoop(ds, stateDeploying, log)
	return ds, sf
}

func (ds *deployState) run() {
	// For clean separation declare and use state as a local variable. eg: `state := ds.startState`
	for ds.state != nil {
		ds.state = ds.state(ds)
	}
}

func (ds *deployState) update(mgroup *manifest.Group) error {
	ds.mux.Lock()
	defer ds.mux.Unlock()

	ds.errch = make(chan error, 1)
	ds.updatech <- mgroup
	return <-ds.errch
}

func (ds *deployState) teardown() error {
	ds.mux.Lock()
	defer ds.mux.Unlock()

	ds.errch = make(chan error, 1)
	ds.teardownch <- struct{}{}
	return <-ds.errch

}

// runLoop provides state transition loop until there are no more states.
// logs transitions between states, and initializes the provided state with
// the deployState struct.
func runLoop(ds *deployState, initState stateFunc, log log.Logger) {
	state := initState
	for state != nil {
		s := state(ds)
		log.With("state", s).Debug("state transitioned")
		state = s
	}
}

// deployStateStart initializes the deployment and waits till it is fully operational
func stateDeploying(ds *deployState) stateFunc {
	if ds.monitor != nil {
		ds.monitor.shutdown() //
		ds.monitor = nil
	}
	// TODO: Pass ds context to Deploy()
	err := ds.client.Deploy(ds.ctx, ds.lease, ds.mgroup)
	if err != nil {
		ds.log.Error("deploying error", err)
		if ds.errch != nil {
			ds.errch <- err
		}
		return stateErrored
	}

	ds.monitor = newDeploymentMonitor(ds.ctx, ds.bus, ds.session, ds.client, ds.lease, ds.mgroup, ds.log)
	return stateNominal
}

// stateNominal is entered when deployment is active, and there is nothing to do
// but wait for signals to perform actions.
func stateNominal(ds *deployState) stateFunc {
	select {
	case mg := <-ds.updatech:
		ds.mgroup = mg
		return stateDeploying
	case <-ds.ctx.Done():
		return stateTearingdown
	case <-ds.teardownch:
		return stateTearingdown
	}
}

// stateErrored engages when an uncrecoverable point has been reached.
func stateErrored(ds *deployState) stateFunc {
	ds.errch = nil // reset the error channel
	// TODO: Determine if there are other steps to take in an errored state.
	return stateNominal
}

// stateTearingdown handles safe shutdown.
func stateTearingdown(ds *deployState) stateFunc {
	if ds.monitor != nil {
		ds.monitor.shutdown()
	}

	err := ds.client.TeardownLease(ds.ctx, ds.lease)
	if err != nil {
		ds.log.Error("error tearing down deployment", err)
		if ds.errch != nil {
			ds.errch <- err
		}
	}

	return nil // exit state machine
}
