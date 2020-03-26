package query

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/ovrclk/akash/x/market/types"
)

// RawClient interface
type RawClient interface {
	Orders() ([]byte, error)
	Bids() ([]byte, error)
	Bid(id types.BidID) ([]byte, error)
	Leases() ([]byte, error)
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

func (c *rawclient) Bids() ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getBidsPath()), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func (c *rawclient) Bid(id types.BidID) ([]byte, error) {
	return []byte{}, fmt.Errorf("TODO: not implemented")
}

func (c *rawclient) Leases() ([]byte, error) {
	buf, _, err := c.ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", c.key, getLeasesPath()), nil)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}
