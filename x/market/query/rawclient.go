package query

import (
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	types "github.com/akash-network/akash-api/go/node/market/v1beta4"
)

// RawClient interface
type RawClient interface {
	Orders(filters OrderFilters) ([]byte, error)
	Order(id types.OrderID) ([]byte, error)
	Bids(filters BidFilters) ([]byte, error)
	Bid(id types.BidID) ([]byte, error)
	Leases(filters LeaseFilters) ([]byte, error)
	Lease(id types.LeaseID) ([]byte, error)
}

// NewRawClient creates a raw client instance with provided context and key
func NewRawClient(ctx sdkclient.Context, key string) RawClient {
	return &rawclient{ctx: ctx, key: key}
}

type rawclient struct {
	ctx sdkclient.Context
	key string
}

func (c *rawclient) Orders(ofilters OrderFilters) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getOrdersPath(ofilters)), nil)
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

func (c *rawclient) Bids(bfilters BidFilters) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getBidsPath(bfilters)), nil)
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

func (c *rawclient) Leases(lfilters LeaseFilters) ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getLeasesPath(lfilters)), nil)
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
