package market

import (
	"errors"

	"github.com/ovrclk/akash/state"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
)

type leaseEngine engine

func newLeaseEngine(log log.Logger) Engine {
	return &leaseEngine{
		log: log,
	}
}

func (e *leaseEngine) Run(state state.State) ([]abci_types.Event, error) {
	var events []abci_types.Event

	leases, err := state.Lease().All()
	if err != nil {
		return events, err
	}

	for _, lease := range leases {
		if levents, err := e.processLease(state, lease); err != nil {
			return events, err
		} else {
			events = append(events, levents...)
		}
	}

	return events, nil
}

// close lease
func (e *leaseEngine) processLease(state state.State, lease *types.Lease) ([]abci_types.Event, error) {
	var events []abci_types.Event

	// skip inactve leases
	if lease.State != types.Lease_ACTIVE {
		return events, nil
	}

	deployment, err := state.Deployment().Get(lease.Deployment)
	if err != nil {
		return events, err
	}
	if deployment == nil {
		return events, errors.New("deployment not found")
	}

	tenant, err := state.Account().Get(deployment.Tenant)
	if err != nil {
		return events, err
	}
	if tenant == nil {
		return events, errors.New("tenant not found")
	}

	// close deployments if tenant has zero balance
	if tenant.Balance == uint64(0) {
		deployment.State = types.Deployment_CLOSED
		if err := state.Deployment().Save(deployment); err != nil {
			return events, err
		}

		events = append(events, eventLeaseClose(lease))
	}

	return events, nil
}

// billing for leases
func ProcessLeases(state appstate.State) error {
	leases, err := state.Lease().All()
	if err != nil {
		return err
	}
	for _, lease := range leases {
		if lease.State == types.Lease_ACTIVE {
			if err := processLease(state, *lease); err != nil {
				return err
			}
		}
	}
	return nil
}

func processLease(state appstate.State, lease types.Lease) error {
	deployment, err := state.Deployment().Get(lease.Deployment)
	if err != nil {
		return err
	}
	if deployment == nil {
		return errors.New("deployment not found")
	}
	tenant, err := state.Account().Get(deployment.Tenant)
	if err != nil {
		return err
	}
	if tenant == nil {
		return errors.New("tenant not found")
	}
	provider, err := state.Provider().Get(lease.Provider)
	if err != nil {
		return err
	}
	if provider == nil {
		return errors.New("provider not found")
	}
	owner, err := state.Account().Get(provider.Owner)
	if err != nil {
		return err
	}
	if owner == nil {
		return errors.New("owner not found")
	}

	p := uint64(lease.Price)

	if tenant.Balance >= p {
		owner.Balance += p
		tenant.Balance -= p
	} else {
		owner.Balance += tenant.Balance
		tenant.Balance = 0
		// TODO: close lease.
	}

	err = state.Account().Save(tenant)
	if err != nil {
		return err
	}

	err = state.Account().Save(owner)
	if err != nil {
		return err
	}

	return nil
}
