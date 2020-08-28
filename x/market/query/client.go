package query

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/ovrclk/akash/x/market/types"
)

// Client interface
type Client interface {
	Orders(filters OrderFilters) (Orders, error)
	Order(id types.OrderID) (Order, error)
	Bids(filters BidFilters) (Bids, error)
	Bid(id types.BidID) (Bid, error)
	Leases(filters LeaseFilters) (Leases, error)
	Lease(id types.LeaseID) (Lease, error)
}

// NewClient creates a client instance with provided context and key
func NewClient(ctx sdkclient.Context, key string) Client {
	return &client{ctx: ctx, key: key}
}

type client struct {
	ctx sdkclient.Context
	key string
}

func (c *client) Orders(ofilters OrderFilters) (Orders, error) {
	var obj Orders
	buf, err := NewRawClient(c.ctx, c.key).Orders(ofilters)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}

func (c *client) Order(id types.OrderID) (Order, error) {
	var obj Order
	buf, err := NewRawClient(c.ctx, c.key).Order(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}

func (c *client) Bids(bfilters BidFilters) (Bids, error) {
	var obj Bids
	buf, err := NewRawClient(c.ctx, c.key).Bids(bfilters)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}

func (c *client) Bid(id types.BidID) (Bid, error) {
	var obj Bid
	buf, err := NewRawClient(c.ctx, c.key).Bid(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}

func (c *client) Leases(lfilters LeaseFilters) (Leases, error) {
	var obj Leases
	buf, err := NewRawClient(c.ctx, c.key).Leases(lfilters)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}

func (c *client) Lease(id types.LeaseID) (Lease, error) {
	var obj Lease
	buf, err := NewRawClient(c.ctx, c.key).Lease(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.LegacyAmino.UnmarshalJSON(buf, &obj)
}
