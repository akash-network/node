package query

import (
	mtypes "pkg.akt.dev/go/node/market/v2beta1"
)

// Client interface
type Client interface {
	Orders(filters OrderFilters) (Orders, error)
	Order(id mtypes.OrderID) (Order, error)
	Bids(filters BidFilters) (Bids, error)
	Bid(id mtypes.BidID) (Bid, error)
	Leases(filters LeaseFilters) (Leases, error)
	Lease(id mtypes.LeaseID) (Lease, error)
}
