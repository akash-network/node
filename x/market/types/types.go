package types

import (
	fmt "fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/validation"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
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
	case OrderMatched:
		return ErrOrderMatched
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
	case OrderMatched:
		return ErrOrderMatched
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

// ValidateCanMatch method validates whether to match order for provided height
func (o Order) ValidateCanMatch(height int64) error {
	if err := o.validateMatchableState(); err != nil {
		return err
	}
	if err := validation.ValidateDeploymentGroup(o.Spec); err != nil {
		return err
	}

	if height < o.StartAt {
		return errors.Wrap(ErrOrderTooEarly, fmt.Sprintf("(%v > %v)", o.StartAt, height))
	}
	if height >= o.CloseAt {
		// Close Open Order if it have surpassed the CloseAt block height.
		return ErrOrderDurationExceeded
	}
	return nil
}

func (o Order) validateMatchableState() error {
	switch o.State {
	case OrderOpen:
		return nil
	case OrderMatched:
		return ErrOrderMatched
	default:
		return ErrOrderClosed
	}
}

// Accept returns whether order filters valid or not
func (filters OrderFilters) Accept(obj Order, stateVal Order_State) bool {
	// Checking owner filter
	if !filters.Owner.Empty() && !filters.Owner.Equals(obj.OrderID.Owner) {
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
	if !filters.Owner.Empty() && !filters.Owner.Equals(obj.BidID.Owner) {
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
	if !filters.Provider.Empty() && !filters.Provider.Equals(obj.BidID.Provider) {
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
	if !filters.Owner.Empty() && !filters.Owner.Equals(obj.LeaseID.Owner) {
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
	if !filters.Provider.Empty() && !filters.Provider.Equals(obj.LeaseID.Provider) {
		return false
	}

	// Checking state filter
	if stateVal != 0 && stateVal != obj.State {
		return false
	}

	return true
}
