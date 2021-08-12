package cluster

import (
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	atypes "github.com/ovrclk/akash/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

func newReservation(order mtypes.OrderID, resources atypes.ResourceGroup) *reservation {
	return &reservation{order: order, resources: resources}
}

type reservation struct {
	order     mtypes.OrderID
	resources atypes.ResourceGroup
	allocated bool
}

var _ ctypes.Reservation = (*reservation)(nil)

func (r *reservation) OrderID() mtypes.OrderID {
	return r.order
}

func (r *reservation) Resources() atypes.ResourceGroup {
	return r.resources
}

func (r *reservation) Allocated() bool {
	return r.allocated
}
