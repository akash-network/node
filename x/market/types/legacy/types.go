package types

import (
	"strings"

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
