package query

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/ovrclk/akash/x/market/types"
)

// Client interface
type Client interface {
	Orders() (Orders, error)
	Bids() (Bids, error)
	Bid(id types.BidID) (Bid, error)
	Leases() (Leases, error)
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
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, OrdersPath()), nil)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Bids() (Bids, error) {
	var obj Bids
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, BidsPath()), nil)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}

func (c *client) Bid(id types.BidID) (Bid, error) {
	return Bid{}, fmt.Errorf("TODO: not implemented")
}

func (c *client) Leases() (Leases, error) {
	var obj Leases
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, LeasesPath()), nil)
	if err != nil {
		return obj, err
	}
	return obj, c.ctx.Codec.UnmarshalJSON(buf, &obj)
}
