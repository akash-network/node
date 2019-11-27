package market

import (
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
)

type deploymentEngine engine

func newDeploymentEngine(log log.Logger) Engine {
	return &deploymentEngine{
		log: log,
	}
}

func (e *deploymentEngine) Run(state state.State) ([]abci_types.Event, error) {
	var events []abci_types.Event
	items, err := state.Deployment().GetMaxRange()
	if err != nil {
		return events, err
	}
	for _, item := range items.Items {
		if devents, err := e.processDeployment(state, item); err != nil {
			return events, err
		} else {
			events = append(events, devents...)
		}
	}
	return events, nil
}

// create orders and leases for deployment
func (e *deploymentEngine) processDeployment(state state.State, deployment types.Deployment) ([]abci_types.Event, error) {
	var events []abci_types.Event

	oseq := state.Deployment().SequenceFor(deployment.Address)
	height := state.Version()

	// skip inactve deployments
	if deployment.State != types.Deployment_ACTIVE {
		return events, nil
	}

	groups, err := state.DeploymentGroup().ForDeployment(deployment.Address)
	if err != nil {
		return events, err
	}

	// process groups
	for _, group := range groups {
		if group.State != types.DeploymentGroup_OPEN {
			continue
		}

		// process current orders
		orders, err := state.Order().ForGroup(group.DeploymentGroupID)
		if err != nil {
			return events, err
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
				event, err := e.processOrder(state, order)
				if err != nil {
					return events, err
				}
				events = append(events, event...)
			}
		}

		// if no active order for the group emit create tx
		if !activeFound {

			order := &types.Order{
				OrderID: types.OrderID{
					Deployment: deployment.Address,
					Group:      group.GetSeq(),
					Seq:        oseq.Advance(),
				},
				EndAt: group.OrderTTL + height,
				State: types.Order_OPEN,
			}

			if err := state.Order().Save(order); err != nil {
				return events, err
			}

			events = append(events, eventOrderCreate(order))
		}
	}

	return events, nil
}

// create leases as necessary
func (e *deploymentEngine) processOrder(state state.State, order *types.Order) ([]abci_types.Event, error) {
	var events []abci_types.Event

	fulfillment, err := matchOrder(state, order)
	if err != nil {
		return events, err
	}
	if fulfillment == nil {
		return events, nil
	}

	lease := &types.Lease{
		LeaseID: fulfillment.LeaseID(),
		Price:   fulfillment.Price,
		State:   types.Lease_ACTIVE,
	}

	// XXX TODO: set all other orders to order.State = types.Order_CLOSED

	order.State = types.Order_MATCHED
	if err := state.Order().Save(order); err != nil {
		return events, err
	}

	if err := state.Lease().Save(lease); err != nil {
		return events, err
	}

	events = append(events, eventLeaseCreate(lease))

	return events, nil
}

func matchOrder(state state.State, order *types.Order) (*types.Fulfillment, error) {
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
