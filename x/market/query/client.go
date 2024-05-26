package query

import (
	types "pkg.akt.dev/go/node/market/v1beta4"
)

// Client interface
type Client interface {
	Orders(filters OrderFilters) (Orders, error)
	Order(id types.OrderID) (Order, error)
	Bids(filters BidFilters) (Bids, error)
	Bid(id types.BidID) (Bid, error)
	Leases(filters LeaseFilters) (Leases, error)
	Lease(id types.LeaseID) (Lease, error)
}
