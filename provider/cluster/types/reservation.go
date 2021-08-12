package cluster

import (
	atypes "github.com/ovrclk/akash/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// Reservation interface implements orders and resources
type Reservation interface {
	OrderID() mtypes.OrderID
	Resources() atypes.ResourceGroup
	Allocated() bool
}
