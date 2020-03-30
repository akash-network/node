package query

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/ovrclk/akash/x/market/types"
)

// RawClient interface
type RawClient interface {
	Orders() ([]byte, error)
	FilterOrders(filters types.OrderFilters) ([]byte, error)
	Order(id types.OrderID) ([]byte, error)
	Bids() ([]byte, error)
	FilterBids(filters types.BidFilters) ([]byte, error)
	Bid(id types.BidID) ([]byte, error)
	Leases() ([]byte, error)
	FilterLeases(filters types.LeaseFilters) ([]byte, error)
	Lease(id types.LeaseID) ([]byte, error)
}

// NewRawClient creates a raw client instance with provided context and key
func NewRawClient(ctx context.CLIContext, key string) RawClient {
	return &rawclient{ctx: ctx, key: key}
}

type rawclient struct {
	ctx context.CLIContext
	key string
}

func (c *rawclient) Orders() ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getOrdersPath()), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) FilterOrders(filters types.OrderFilters) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getOrdersFilterPath(filters)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) Order(id types.OrderID) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, OrderPath(id)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) Bids() ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getBidsPath()), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) FilterBids(filters types.BidFilters) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getBidsFilterPath(filters)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) Bid(id types.BidID) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getBidPath(id)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) Leases() ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getLeasesPath()), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) FilterLeases(filters types.LeaseFilters) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getLeasesFilterPath(filters)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) Lease(id types.LeaseID) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, LeasePath(id)), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}
