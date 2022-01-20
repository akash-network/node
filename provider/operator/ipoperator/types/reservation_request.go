package types

import mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"

type IPReservationRequest struct {
	OrderID  mtypes.OrderID
	Quantity uint
}
