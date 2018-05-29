package cluster

import "github.com/ovrclk/akash/types"

type Reservation interface {
	OrderID() types.OrderID
	Group() *types.DeploymentGroup
}

func newReservation(order types.OrderID, group *types.DeploymentGroup) Reservation {
	return &reservation{order, group}
}

type reservation struct {
	order types.OrderID
	group *types.DeploymentGroup
}

func (r *reservation) OrderID() types.OrderID {
	return r.order
}

func (r *reservation) Group() *types.DeploymentGroup {
	return r.group
}
