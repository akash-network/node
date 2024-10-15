package query

import (
	"pkg.akt.dev/go/node/market/v1"
)

// Client interface
type Client interface {
	Orders(filters OrderFilters) (Orders, error)
	Order(id v1.OrderID) (Order, error)
	Bids(filters BidFilters) (Bids, error)
	Bid(id v1.BidID) (Bid, error)
	Leases(filters LeaseFilters) (Leases, error)
	Lease(id v1.LeaseID) (Lease, error)
}
