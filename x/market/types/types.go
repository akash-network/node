package types

import (
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/types"
	atypes "github.com/ovrclk/akash/x/audit/types"

	"gopkg.in/yaml.v3"
)

// ID method returns OrderID details of specific order
func (o Order) ID() OrderID {
	return o.OrderID
}

// String implements the Stringer interface for a Order object.
func (o Order) String() string {
	out, _ := yaml.Marshal(o)
	return string(out)
}

// Orders is a collection of Order
type Orders []Order

// String implements the Stringer interface for a Orders object.
func (o Orders) String() string {
	var out string
	for _, order := range o {
		out += order.String() + "\n"
	}

	return strings.TrimSpace(out)
}

// ValidateCanBid method validates whether order is open or not and
// returns error if not
func (o Order) ValidateCanBid() error {
	switch o.State {
	case OrderOpen:
		return nil
	case OrderActive:
		return ErrOrderActive
	default:
		return ErrOrderClosed
	}
}

// ValidateInactive method validates whether order is open or not and
// returns error if not
func (o Order) ValidateInactive() error {
	switch o.State {
	case OrderClosed:
		return nil
	case OrderActive:
		return ErrOrderActive
	default:
		return ErrOrderClosed
	}
}

// Price method returns price of specific order
func (o Order) Price() sdk.Coin {
	return o.Spec.Price()
}

// MatchAttributes method compares provided attributes with specific order attributes
func (o Order) MatchAttributes(attrs []types.Attribute) bool {
	return o.Spec.MatchAttributes(attrs)
}

// MatchRequirements method compares provided attributes with specific order attributes
func (o Order) MatchRequirements(prov []atypes.Provider) bool {
	return o.Spec.MatchRequirements(prov)
}

// MatchResourcesRequirements method compares provider capabilities with specific order resources attributes
func (o Order) MatchResourcesRequirements(attr types.Attributes) bool {
	return o.Spec.MatchResourcesRequirements(attr)
}

// Accept returns whether order filters valid or not
func (filters OrderFilters) Accept(obj Order, stateVal Order_State) bool {
	// Checking owner filter
	if filters.Owner != "" && filters.Owner != obj.OrderID.Owner {
		return false
	}

	// Checking dseq filter
	if filters.DSeq != 0 && filters.DSeq != obj.OrderID.DSeq {
		return false
	}

	// Checking gseq filter
	if filters.GSeq != 0 && filters.GSeq != obj.OrderID.GSeq {
		return false
	}

	// Checking oseq filter
	if filters.OSeq != 0 && filters.OSeq != obj.OrderID.OSeq {
		return false
	}

	// Checking state filter
	if stateVal != 0 && stateVal != obj.State {
		return false
	}

	return true
}

// ID method returns BidID details of specific bid
func (obj Bid) ID() BidID {
	return obj.BidID
}

// String implements the Stringer interface for a Bid object.
func (obj Bid) String() string {
	out, _ := yaml.Marshal(obj)
	return string(out)
}

// Bids is a collection of Bid
type Bids []Bid

// String implements the Stringer interface for a Bids object.
func (b Bids) String() string {
	var out string
	for _, bid := range b {
		out += bid.String() + "\n"
	}

	return strings.TrimSpace(out)
}

// Accept returns whether bid filters valid or not
func (filters BidFilters) Accept(obj Bid, stateVal Bid_State) bool {
	// Checking owner filter
	if filters.Owner != "" && filters.Owner != obj.BidID.Owner {
		return false
	}

	// Checking dseq filter
	if filters.DSeq != 0 && filters.DSeq != obj.BidID.DSeq {
		return false
	}

	// Checking gseq filter
	if filters.GSeq != 0 && filters.GSeq != obj.BidID.GSeq {
		return false
	}

	// Checking oseq filter
	if filters.OSeq != 0 && filters.OSeq != obj.BidID.OSeq {
		return false
	}

	// Checking provider filter
	if filters.Provider != "" && filters.Provider != obj.BidID.Provider {
		return false
	}

	// Checking state filter
	if stateVal != 0 && stateVal != obj.State {
		return false
	}

	return true
}

// ID method returns LeaseID details of specific lease
func (obj Lease) ID() LeaseID {
	return obj.LeaseID
}

// String implements the Stringer interface for a Lease object.
func (obj Lease) String() string {
	out, _ := yaml.Marshal(obj)
	return string(out)
}

// Leases is a collection of Lease
type Leases []Lease

// String implements the Stringer interface for a Leases object.
func (l Leases) String() string {
	var out string
	for _, order := range l {
		out += order.String() + "\n"
	}

	return strings.TrimSpace(out)
}

// Accept returns whether lease filters valid or not
func (filters LeaseFilters) Accept(obj Lease, stateVal Lease_State) bool {
	// Checking owner filter
	if filters.Owner != "" && filters.Owner != obj.LeaseID.Owner {
		return false
	}

	// Checking dseq filter
	if filters.DSeq != 0 && filters.DSeq != obj.LeaseID.DSeq {
		return false
	}

	// Checking gseq filter
	if filters.GSeq != 0 && filters.GSeq != obj.LeaseID.GSeq {
		return false
	}

	// Checking oseq filter
	if filters.OSeq != 0 && filters.OSeq != obj.LeaseID.OSeq {
		return false
	}

	// Checking provider filter
	if filters.Provider != "" && filters.Provider != obj.LeaseID.Provider {
		return false
	}

	// Checking state filter
	if stateVal != 0 && stateVal != obj.State {
		return false
	}

	return true
}
