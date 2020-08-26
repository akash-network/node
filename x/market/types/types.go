package types

import (
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// //go:generate stringer -linecomment -output=autogen_stringer.go -type=OrderState,BidState,LeaseState

// // OrderState defines state of order
// type OrderState uint32

// const (
// 	// OrderOpen is used when state of order is open
// 	OrderOpen OrderState = iota + 1 // open
// 	// OrderMatched is used when state of order is matched
// 	OrderMatched // matched
// 	// OrderClosed is used when state of order is closed
// 	OrderClosed // closed
// )

// // OrderStateMap is used to decode order state flag value
// var OrderStateMap = map[string]OrderState{
// 	"open":    OrderOpen,
// 	"matched": OrderMatched,
// 	"closed":  OrderClosed,
// }

// Order stores orderID, state of order and other details
// type Order struct {
// 	OrderID `json:"id"`
// 	State   OrderState `json:"state"`

// 	// block height to start matching
// 	StartAt int64            `json:"start-at"`
// 	Spec    dtypes.GroupSpec `json:"spec"`
// }

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
func (o Order) MatchAttributes(attrs []sdk.Attribute) bool {
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
func (filters OrderFilters) Accept(obj Order) bool {
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
	if filters.State != 0 && filters.State != obj.State {
		return false
	}

	return true
}

// // BidState defines state of bid
// type BidState uint32

// const (
// 	// BidOpen is used when state of bid is opened
// 	BidOpen BidState = iota + 1 // open
// 	// BidMatched is used when state of bid is matched
// 	BidMatched // matched
// 	// BidLost is used when state of bid is lost
// 	BidLost // lost
// 	// BidClosed is used when state of bid is closed
// 	BidClosed // closed
// )

// // BidStateMap is used to decode bid state flag value
// var BidStateMap = map[string]BidState{
// 	"open":    BidOpen,
// 	"matched": BidMatched,
// 	"lost":    BidLost,
// 	"closed":  BidClosed,
// }

// Bid stores BidID, state of bid and price
// type Bid struct {
// 	BidID `json:"id"`
// 	State BidState `json:"state"`
// 	Price sdk.Coin `json:"price"`
// }

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
func (filters BidFilters) Accept(obj Bid) bool {
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
	if filters.State != 0 && filters.State != obj.State {
		return false
	}

	return true
}

// // LeaseState defines state of Lease
// type LeaseState uint32

// const (
// 	// LeaseActive is used when state of lease is active
// 	LeaseActive LeaseState = iota + 1 // active
// 	// LeaseInsufficientFunds is used when lease has insufficient funds
// 	LeaseInsufficientFunds // insufficient_funds
// 	// LeaseClosed is used when state of lease is closed
// 	LeaseClosed // closed
// )

// // LeaseStateMap is used to decode lease state flag value
// var LeaseStateMap = map[string]LeaseState{
// 	"active":             LeaseActive,
// 	"insufficient_funds": LeaseInsufficientFunds,
// 	"closed":             LeaseClosed,
// }

// Lease stores LeaseID, state of lease and price
// type Lease struct {
// 	LeaseID `json:"id"`
// 	State   LeaseState `json:"state"`
// 	Price   sdk.Coin   `json:"price"`
// }

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
func (filters LeaseFilters) Accept(obj Lease) bool {
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
	if filters.State != 0 && filters.State != obj.State {
		return false
	}

	return true
}
