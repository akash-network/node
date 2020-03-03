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

// OrdersPath returns orders path for queries
func OrdersPath() string {
	return ordersPath
}

// OrderPath return order path of given order id for queries
func OrderPath(id types.OrderID) string {
	return fmt.Sprintf("%s/%s", orderPath, orderParts(id))
}

//BidsPath returns bids path for queries
func BidsPath() string {
	return bidsPath
}

// BidPath return bid path of given bid id for queries
func BidPath(id types.BidID) string {
	return fmt.Sprintf("%s/%s/%s", bidPath, orderParts(id.OrderID()), id.Provider)
}

// LeasesPath returns leases path for queries
func LeasesPath() string {
	return leasesPath
}

// LeasePath return lease path of given lease id for queries
func LeasePath(id types.LeaseID) string {
	return fmt.Sprintf("%s/%s/%s", leasePath, orderParts(id.OrderID()), id.Provider)
}

func orderParts(id types.OrderID) string {
	return fmt.Sprintf("%s/%v/%v/%v", id.Owner, id.DSeq, id.GSeq, id.OSeq)
}
