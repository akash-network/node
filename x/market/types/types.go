package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
)

//go:generate stringer -linecomment -output=autogen_stringer.go -type=OrderState,BidState,LeaseState

// OrderState defines state of order
type OrderState uint32

const (
	// OrderOpen is used when state of order is open
	OrderOpen OrderState = iota + 1 // order
	// OrderMatched is used when state of order is matched
	OrderMatched // matched
	// OrderClosed is used when state of order is closed
	OrderClosed // closed
)

// OrderStateMap is used to decode order state flag value
var OrderStateMap = map[string]OrderState{
	"open":    OrderOpen,
	"matched": OrderMatched,
	"closed":  OrderClosed,
}

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

	if o.StartAt > height {
		return errors.Errorf("too early to match order (%v > %v)", o.StartAt, height)
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

// BidState defines state of bid
type BidState uint32

const (
	// BidOpen is used when state of bid is opened
	BidOpen BidState = iota + 1 // open
	// BidMatched is used when state of bid is matched
	BidMatched // matched
	// BidLost is used when state of bid is lost
	BidLost // lost
	// BidClosed is used when state of bid is closed
	BidClosed // closed
)

// BidStateMap is used to decode bid state flag value
var BidStateMap = map[string]BidState{
	"open":    BidOpen,
	"matched": BidMatched,
	"lost":    BidLost,
	"closed":  BidClosed,
}

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

// LeaseState defines state of Lease
type LeaseState uint32

const (
	// LeaseActive is used when state of lease is active
	LeaseActive LeaseState = iota + 1 // active
	// LeaseInsufficientFunds is used when lease has insufficient funds
	LeaseInsufficientFunds // insufficient-funds
	// LeaseClosed is used when state of lease is closed
	LeaseClosed // closed
)

// LeaseStateMap is used to decode lease state flag value
var LeaseStateMap = map[string]LeaseState{
	"active":             LeaseActive,
	"insufficient-funds": LeaseInsufficientFunds,
	"closed":             LeaseClosed,
}

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
