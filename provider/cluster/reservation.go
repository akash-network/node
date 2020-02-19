package cluster

import (
	atypes "github.com/ovrclk/akash/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type Reservation interface {
	OrderID() mtypes.OrderID
	Resources() atypes.ResourceGroup
}

func newReservation(order mtypes.OrderID, resources atypes.ResourceGroup) *reservation {
	return &reservation{order: order, resources: resources}
}

type reservation struct {
	order     mtypes.OrderID
	resources atypes.ResourceGroup
	allocated bool
}

func (r *reservation) OrderID() mtypes.OrderID {
	return r.order
}

func (r *reservation) Resources() atypes.ResourceGroup {
	return r.resources
}
