package market

import (
	"errors"

	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tendermint/libs/log"
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

	if err := e.processLeases(state, buf); err != nil {
		return buf.all(), err
	}

	return buf.all(), nil
}

// close leases as necessary
func (e engine) processLeases(state state.State, w txBuffer) error {
	leases, err := state.Lease().All()
	if err != nil {
		return err
	}
	for _, lease := range leases {
		if err := e.processLease(state, w, lease); err != nil {
			return err
		}
	}
	return nil
}

// close lease
func (e engine) processLease(state state.State, w txBuffer, lease *types.Lease) error {

	// skip inactve leases
	if lease.State != types.Lease_ACTIVE {
		return nil
	}

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

	// close deployments if tenant has zero balance
	if tenant.Balance == uint64(0) {
		w.put(&types.TxCloseDeployment{
			Deployment: deployment.Address,
			Reason:     types.TxCloseDeployment_INSUFFICIENT,
		})
	}

	return nil
}

// create orders and leases as necessary
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

// create orders and leases for deployment
func (e engine) processDeployment(state state.State, w txBuffer, deployment types.Deployment) error {

	nextSeq := state.Deployment().SequenceFor(deployment.Address).Next()
	height := state.Version()

	// skip inactve deployments
	if deployment.State != types.Deployment_ACTIVE {
		return nil
	}

	groups, err := state.DeploymentGroup().ForDeployment(deployment.Address)
	if err != nil {
		return err
	}

	// process groups
	for _, group := range groups {
		if group.State != types.DeploymentGroup_OPEN {
			continue
		}

		// process current orders
		orders, err := state.Order().ForGroup(group.DeploymentGroupID)
		if err != nil {
			return err
		}

		// no active orders found for the deployment group
		activeFound := false

		// for each order for the deployment group
		for _, order := range orders {
			// try to create a lease for the order
			if !activeFound && order.State == types.Order_OPEN || order.State == types.Order_MATCHED {
				activeFound = true
			}
			if order.State == types.Order_OPEN && order.EndAt <= height {
				err := e.processOrder(state, w, order)
				if err != nil {
					return err
				}
			}
		}

		// if no active order for the group emit create tx
		if !activeFound {
			w.put(&types.TxCreateOrder{
				OrderID: types.OrderID{
					Deployment: deployment.Address,
					Group:      group.GetSeq(),
					Seq:        nextSeq,
				},
				EndAt: group.OrderTTL + height,
			})
			nextSeq++
		}
	}

	return nil
}

func BestFulfillment(state state.State, order *types.Order) (*types.Fulfillment, error) {
	fulfillments, err := state.Fulfillment().ForOrder(order.OrderID)
	if err != nil {
		return nil, err
	}

	// no orders to match
	if len(fulfillments) == 0 {
		return nil, nil
	}

	// match with cheapest order
	bestMatch := 0
	found := false
	for i, fulfillment := range fulfillments {
		if fulfillment.State == types.Fulfillment_OPEN {
			found = true
			if fulfillment.Price < fulfillments[bestMatch].Price {
				bestMatch = i
			}
		}
	}

	// no orders to match
	if !found {
		return nil, nil
	}

	return fulfillments[bestMatch], nil
}

// create leases as necessary
func (e engine) processOrder(state state.State, w txBuffer, order *types.Order) error {

	fulfillment, err := BestFulfillment(state, order)
	if err != nil {
		return err
	}
	if fulfillment == nil {
		return nil
	}

	w.put(&types.TxCreateLease{
		LeaseID: fulfillment.LeaseID(),
		Price:   fulfillment.Price,
	})

	return nil
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
