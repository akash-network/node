package cluster

import (
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"

	"github.com/ovrclk/akash/provider/cluster/util"

	atypes "github.com/ovrclk/akash/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

func newReservation(order mtypes.OrderID, resources atypes.ResourceGroup) *reservation {
	return &reservation{
		order:            order,
		resources:        resources,
		endpointQuantity: util.GetEndpointQuantityOfResourceGroup(resources, atypes.Endpoint_LEASED_IP)}
}

type reservation struct {
	order            mtypes.OrderID
	resources        atypes.ResourceGroup
	allocated        bool
	endpointQuantity uint
	ipsConfirmed     bool
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
