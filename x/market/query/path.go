package query

import (
	"fmt"

	"github.com/ovrclk/akash/x/market/types"
)

const (
	ordersPath = "orders"
	orderPath  = "order"
	bidsPath   = "bids"
	bidPath    = "bid"
	leasesPath = "leases"
	leasePath  = "lease"
)

func OrdersPath() string {
	return ordersPath
}

func OrderPath(id types.OrderID) string {
	return fmt.Sprintf("%s/%s", orderPath, orderParts(id))
}

func BidsPath() string {
	return bidsPath
}

func BidPath(id types.BidID) string {
	return fmt.Sprintf("%s/%s/%s", bidPath, orderParts(id.OrderID()), id.Provider)
}

func LeasesPath() string {
	return leasesPath
}

func LeasePath(id types.LeaseID) string {
	return fmt.Sprintf("%s/%s/%s", leasePath, orderParts(id.OrderID()), id.Provider)
}

func orderParts(id types.OrderID) string {
	return fmt.Sprintf("%s/%v/%v/%v", id.Owner, id.DSeq, id.GSeq, id.OSeq)
}
