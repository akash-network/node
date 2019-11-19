package query

import "github.com/ovrclk/akash/x/market/types"

type (
	Order  types.Order
	Orders []Order

	Bid  types.Bid
	Bids []Bid

	Lease  types.Lease
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
