package cluster

import "github.com/ovrclk/akash/types"

type Reservation interface {
	OrderID() types.OrderID
	Resources() types.ResourceList
}

func newReservation(order types.OrderID, resources types.ResourceList) *reservation {
	return &reservation{order: order, resources: resources}
}

type reservation struct {
	order     types.OrderID
	resources types.ResourceList
	allocated bool
}

func (r *reservation) OrderID() types.OrderID {
	return r.order
}

func (r *reservation) Resources() types.ResourceList {
	return r.resources
}
