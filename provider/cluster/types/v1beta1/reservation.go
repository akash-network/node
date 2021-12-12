package v1beta1

import (
	atypes "github.com/ovrclk/akash/types/v1beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta1"
)

// Reservation interface implements orders and resources
type Reservation interface {
	OrderID() mtypes.OrderID
	Resources() atypes.ResourceGroup
	Allocated() bool
}
