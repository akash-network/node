package query

import "github.com/ovrclk/akash/x/market/types"

type (
	//Order type
	Order types.Order
	//Orders - Slice of Order Struct
	Orders []Order

	// Bid type
	Bid types.Bid
	// Bids - Slice of Bid Struct
	Bids []Bid

	//Lease type
	Lease types.Lease
	// Leases - Slice of Lease Struct
	Leases []Lease
)

func (obj Order) String() string {
	return "TODO see deployment/query/types.go"
}
func (obj Orders) String() string {
	return "TODO see deployment/query/types.go"
}

func (obj Bid) String() string {
	return "TODO see deployment/query/types.go"
}
func (obj Bids) String() string {
	return "TODO see deployment/query/types.go"
}

func (obj Lease) String() string {
	return "TODO see deployment/query/types.go"
}
func (obj Leases) String() string {
	return "TODO see deployment/query/types.go"
}
