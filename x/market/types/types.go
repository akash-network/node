package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	tmkv "github.com/tendermint/tendermint/libs/kv"
)

// OrderState defines state of order
type OrderState uint8

const (
	// OrderOpen is used when state of order is open
	OrderOpen OrderState = iota + 1
	// OrderMatched is used when state of order is matched
	OrderMatched
	// OrderClosed is used when state of order is closed
	OrderClosed
)

// OrderFilters defines flags for order list filter
type OrderFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from OrderStateMap
	State OrderState
}

// OrderStateMap is used to decode order state flag value
var OrderStateMap = map[string]OrderState{
	"open":    OrderOpen,
	"matched": OrderMatched,
	"closed":  OrderClosed,
}

// Order stores orderID, state of order and other details
type Order struct {
	OrderID `json:"id"`
	State   OrderState `json:"state"`

	// block height to start matching
	StartAt int64            `json:"start-at"`
	Spec    dtypes.GroupSpec `json:"spec"`
}

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
		return fmt.Errorf("order matched")
	default:
		return fmt.Errorf("order closed")
	}
}

// Price method returns price of specific order
func (o Order) Price() sdk.Coin {
	return o.Spec.Price()
}

// MatchAttributes method compares provided attributes with specific order attributes
func (o Order) MatchAttributes(attrs []tmkv.Pair) bool {
	return o.Spec.MatchAttributes(attrs)
}

// ValidateCanMatch method validates whether to match order for provided height
func (o Order) ValidateCanMatch(height int64) error {
	if err := o.validateMatchableState(); err != nil {
		return err
	}

	if o.StartAt > height {
		return fmt.Errorf("too early to match order (%v > %v)", o.StartAt, height)
	}
	return nil
}

func (o Order) validateMatchableState() error {
	switch o.State {
	case OrderOpen:
		return nil
	case OrderMatched:
		return fmt.Errorf("order matched")
	default:
		return fmt.Errorf("order closed")
	}
}

// BidState defines state of bid
type BidState uint8

const (
	// BidOpen is used when state of bid is opened
	BidOpen BidState = iota + 1
	// BidMatched is used when state of bid is matched
	BidMatched
	// BidLost is used when state of bid is lost
	BidLost
	// BidClosed is used when state of bid is closed
	BidClosed
)

// BidFilters defines flags for bid list filter
type BidFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from BidStateMap
	State BidState
}

// BidStateMap is used to decode bid state flag value
var BidStateMap = map[string]BidState{
	"open":    BidOpen,
	"matched": BidMatched,
	"lost":    BidLost,
	"closed":  BidClosed,
}

// Bid stores BidID, state of bid and price
type Bid struct {
	BidID `json:"id"`
	State BidState `json:"state"`
	Price sdk.Coin `json:"price"`
}

// ID method returns BidID details of specific bid
func (obj Bid) ID() BidID {
	return obj.BidID
}

// LeaseState defines state of Lease
type LeaseState uint8

const (
	// LeaseActive is used when state of lease is active
	LeaseActive LeaseState = iota + 1
	// LeaseInsufficientFunds is used when lease has insufficient funds
	LeaseInsufficientFunds
	// LeaseClosed is used when state of lease is closed
	LeaseClosed
)

// LeaseFilters defines flags for lease list filter
type LeaseFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from LeaseStateMap
	State LeaseState
}

// LeaseStateMap is used to decode lease state flag value
var LeaseStateMap = map[string]LeaseState{
	"active":       LeaseActive,
	"insufficient": LeaseInsufficientFunds,
	"closed":       LeaseClosed,
}

// Lease stores LeaseID, state of lease and price
type Lease struct {
	LeaseID `json:"id"`
	State   LeaseState `json:"state"`
	Price   sdk.Coin   `json:"price"`
}

// ID method returns LeaseID details of specific lease
func (obj Lease) ID() LeaseID {
	return obj.LeaseID
}
