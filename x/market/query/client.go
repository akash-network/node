package query

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/ovrclk/akash/x/market/types"
)

// Client interface
type Client interface {
	Orders() (Orders, error)
	Order(id types.OrderID) (Order, error)
	Bids() (Bids, error)
	Bid(id types.BidID) (Bid, error)
	Leases() (Leases, error)
	Lease(id types.LeaseID) (Lease, error)
}

// NewClient creates a client instance with provided context and key
func NewClient(ctx context.CLIContext, key string) Client {
	return &client{ctx: ctx, key: key}
}

type client struct {
	ctx context.CLIContext
	key string
}

func (c *client) Orders() (Orders, error) {
	var obj Orders
	buf, err := NewRawClient(c.ctx, c.key).Orders()
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Order(id types.OrderID) (Order, error) {
	var obj Order
	buf, err := NewRawClient(c.ctx, c.key).Order(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Bids() (Bids, error) {
	var obj Bids
	buf, err := NewRawClient(c.ctx, c.key).Bids()
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Bid(id types.BidID) (Bid, error) {
	var obj Bid
	buf, err := NewRawClient(c.ctx, c.key).Bid(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Leases() (Leases, error) {
	var obj Leases
	buf, err := NewRawClient(c.ctx, c.key).Leases()
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Lease(id types.LeaseID) (Lease, error) {
	var obj Lease
	buf, err := NewRawClient(c.ctx, c.key).Lease(id)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}
