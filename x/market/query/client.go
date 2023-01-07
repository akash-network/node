package query

import (
	types "github.com/akash-network/node/x/market/types/v1beta2"
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
