package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	tmkv "github.com/tendermint/tendermint/libs/kv"
)

type OrderState uint8

const (
	OrderOpen    OrderState = iota
	OrderMatched OrderState = iota
	OrderClosed  OrderState = iota
)

type Order struct {
	OrderID `json:"id"`
	State   OrderState `json:"state"`

	// block height to start matching
	StartAt int64            `json:"start-at"`
	Spec    dtypes.GroupSpec `json:"spec"`
}

func (obj Order) ID() OrderID {
	return obj.OrderID
}

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

func (o Order) Price() sdk.Coin {
	return o.Spec.Price()
}

func (o Order) MatchAttributes(attrs []tmkv.Pair) bool {
	return o.Spec.MatchAttributes(attrs)
}

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

type BidState uint8

const (
	BidOpen    BidState = iota
	BidMatched BidState = iota
	BidLost    BidState = iota
	BidClosed  BidState = iota
)

type Bid struct {
	BidID `json:"id"`
	State BidState `json:"state"`
	Price sdk.Coin `json:"price"`
}

func (obj Bid) ID() BidID {
	return obj.BidID
}

type LeaseState uint8

const (
	LeaseActive            LeaseState = iota
	LeaseInsufficientFunds LeaseState = iota
	LeaseClosed            LeaseState = iota
)

type Lease struct {
	LeaseID `json:"id"`
	State   LeaseState `json:"state"`
	Price   sdk.Coin   `json:"price"`
}

func (obj Lease) ID() LeaseID {
	return obj.LeaseID
}
