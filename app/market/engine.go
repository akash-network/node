package market

import (
	"github.com/ovrclk/akash/state"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
)

type Engine interface {
	Run(state state.State) ([]abci_types.Event, error)
}

type engine struct {
	log log.Logger
}

func NewEngine(log log.Logger) Engine {
	return &engine{log: log}
}

func (e engine) Run(state state.State) ([]abci_types.Event, error) {
	var events []abci_types.Event

	// Process active leases
	//   * transfer tokens from tenant to provider
	//   * close if insufficient funds

	{
		eevents, err := newLeaseEngine(e.log.With("engine-cmp", "lease")).Run(state)
		if err != nil {
			return events, err
		}
		events = append(events, eevents...)
	}

	// Process active deployments
	//   * Create deployment orders as necessary
	//   * Create leases as necessary

	{
		eevents, err := newDeploymentEngine(e.log.With("engine-cmp", "deployments")).Run(state)
		if err != nil {
			return events, err
		}
		events = append(events, eevents...)
	}

	return events, nil
}
