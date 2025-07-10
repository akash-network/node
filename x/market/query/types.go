package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/node/market/v1beta5"
)

type (
	// Order type
	Order v1beta5.Order
	// Orders - Slice of Order Struct
	Orders []Order

	// Bid type
	Bid v1beta5.Bid
	// Bids - Slice of Bid Struct
	Bids []Bid

	// Lease type
	Lease v1.Lease
	// Leases - Slice of Lease Struct
	Leases []Lease
)

const (
	todo = "TODO see deployment/query/types.go"
)

// OrderFilters defines flags for order list filter
type OrderFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from Order_State_value
	State v1beta5.Order_State
}

// BidFilters defines flags for bid list filter
type BidFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from Bid_State_value
	State v1beta5.Bid_State
}

// LeaseFilters defines flags for lease list filter
type LeaseFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from Lease_State_value
	State v1.Lease_State
}

// Accept returns true if object matches filter requirements
func (f OrderFilters) Accept(obj v1beta5.Order, isValidState bool) bool {
	if (f.Owner.Empty() && !isValidState) ||
		(f.Owner.Empty() && (obj.State == f.State)) ||
		(!isValidState && obj.ID.Owner == f.Owner.String()) ||
		(obj.ID.Owner == f.Owner.String() && obj.State == f.State) {
		return true
	}

	return false
}

// Accept returns true if object matches filter requirements
func (f BidFilters) Accept(obj v1beta5.Bid, isValidState bool) bool {
	if (f.Owner.Empty() && !isValidState) ||
		(f.Owner.Empty() && (obj.State == f.State)) ||
		(!isValidState && obj.ID.Owner == f.Owner.String()) ||
		(obj.ID.Owner == f.Owner.String() && obj.State == f.State) {
		return true
	}

	return false
}

// Accept returns true if object matches filter requirements
func (f LeaseFilters) Accept(obj v1.Lease, isValidState bool) bool {
	if (f.Owner.Empty() && !isValidState) ||
		(f.Owner.Empty() && (obj.State == f.State)) ||
		(!isValidState && (obj.ID.Owner == f.Owner.String())) ||
		(obj.ID.Owner == f.Owner.String() && obj.State == f.State) {
		return true
	}
	return false
}

func (obj Order) String() string {
	return todo
}

func (obj Orders) String() string {
	return todo
}

func (obj Bid) String() string {
	return todo
}

func (obj Bids) String() string {
	return todo
}

func (obj Lease) String() string {
	return todo
}

func (obj Leases) String() string {
	return todo
}
