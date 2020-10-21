package query

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client/context"
)

// RawClient interface
type RawClient interface {
	CirculatingSupply() ([]byte, error)
}

// NewRawClient creates a client instance with provided context and key
func NewRawClient(ctx context.CLIContext, key string) RawClient {
	return &rawclient{ctx: ctx, key: key}
}

type rawclient struct {
	ctx context.CLIContext
	key string
}

func (c *rawclient) CirculatingSupply() ([]byte, error) {
	buf, _, err := c.ctx.Query(fmt.Sprintf("custom/%s/%s", c.key, getCirculatingPath()))
	if err != nil {
		return []byte{}, err
	}
	return buf, err
}
