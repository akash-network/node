package cluster

import (
	atypes "github.com/ovrclk/akash/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

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
