package market

import (
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
)

type Engine interface {
	Run(state state.State) ([]interface{}, error)
}

type engine struct {
	log log.Logger
}

func NewEngine(log log.Logger) Engine {
	return &engine{log: log}
}

func (e engine) Run(state state.State) ([]interface{}, error) {
	buf := &txBuffer_{}

	if err := e.processDeployments(state, buf); err != nil {
		return buf.all(), err
	}

	return buf.all(), nil
}

// create orders as necessary
func (e engine) processDeployments(state state.State, w txBuffer) error {
	items, err := state.Deployment().GetMaxRange()
	if err != nil {
		return err
	}
	for _, item := range items.Items {
		if err := e.processDeployment(state, w, item); err != nil {
			return err
		}
	}
	return nil
}

func (e engine) processDeployment(state state.State, w txBuffer, deployment types.Deployment) error {

	nextSeq := state.Deployment().SequenceFor(deployment.Address).Next()

	if deployment.State != types.Deployment_ACTIVE {
		return nil
	}

	groups, err := state.DeploymentGroup().ForDeployment(deployment.Address)
	if err != nil {
		return err
	}

	for _, group := range groups {
		if group.State != types.DeploymentGroup_OPEN {
			continue
		}

		// TODO: put ttl on orders
		// TODO: cancel stale orders

		// process current orders
		orders, err := state.Order().ForGroup(group)
		if err != nil {
			return err
		}

		activeFound := false

		for _, order := range orders {
			active, err := e.processOrder(state, w, order)
			if err != nil {
				return err
			}
			if !activeFound && active {
				activeFound = true
			}
		}

		// if no active order emit create tx
		if !activeFound {
			w.put(&types.TxCreateOrder{
				Order: &types.Order{
					Deployment: deployment.Address,
					Group:      group.GetSeq(),
					Order:      nextSeq,
					State:      types.Order_OPEN,
				},
			})
			nextSeq++
		}
	}

	return nil
}

// create leases as necessary
func (e engine) processOrder(state state.State, w txBuffer, dorder *types.Order) (bool, error) {

	switch dorder.State {
	case types.Order_CLOSED:
		return false, nil
	case types.Order_MATCHED:
		return true, nil
	}

	forders, err := state.Fulfillment().ForOrder(dorder)
	if err != nil {
		return true, err
	}

	// no orders to match
	if len(forders) == 0 {
		return true, nil
	}

	// match with cheapest order
	bestMatch := 0
	for i, fulfillment := range forders {
		if fulfillment.Price < forders[bestMatch].Price {
			bestMatch = i
		}
	}

	forder := forders[bestMatch]

	w.put(&types.TxCreateLease{
		Lease: &types.Lease{
			Deployment: forder.Deployment,
			Group:      forder.Group,
			Order:      forder.Order,
			Provider:   forder.Provider,
		},
	})

	return true, nil
}

type txBuffer interface {
	put(tx interface{})
	all() []interface{}
}

type txBuffer_ struct {
	txs []interface{}
}

func (b *txBuffer_) put(tx interface{}) {
	b.txs = append(b.txs, tx)
}

func (b *txBuffer_) all() []interface{} {
	return b.txs
}
